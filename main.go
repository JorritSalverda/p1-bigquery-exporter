package main

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kingpin"
	foundation "github.com/estafette/estafette-foundation"
	"github.com/rs/zerolog/log"
	"github.com/tarm/serial"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	// set when building the application
	appgroup  string
	app       string
	version   string
	branch    string
	revision  string
	buildDate string
	goVersion = runtime.Version()

	// application specific config
	p1DevicePath = kingpin.Flag("p1-device-path", "Path to usb device connecting P1 smart meter.").Default("/dev/ttyUSB0").OverrideDefaultFromEnvar("P1_DEVICE_PATH").String()

	bigqueryEnable    = kingpin.Flag("bigquery-enable", "Toggle to enable or disable bigquery integration").Default("true").OverrideDefaultFromEnvar("BQ_ENABLE").Bool()
	bigqueryProjectID = kingpin.Flag("bigquery-project-id", "Google Cloud project id that contains the BigQuery dataset").Envar("BQ_PROJECT_ID").Required().String()
	bigqueryDataset   = kingpin.Flag("bigquery-dataset", "Name of the BigQuery dataset").Envar("BQ_DATASET").Required().String()
	bigqueryTable     = kingpin.Flag("bigquery-table", "Name of the BigQuery table").Envar("BQ_TABLE").Required().String()

	configPath                   = kingpin.Flag("config-path", "Path to the config.yaml file").Default("/configs/config.yaml").OverrideDefaultFromEnvar("CONFIG_PATH").String()
	measurementFilePath          = kingpin.Flag("state-file-path", "Path to file with state.").Default("/configs/last-measurement.json").OverrideDefaultFromEnvar("MEASUREMENT_FILE_PATH").String()
	measurementFileConfigMapName = kingpin.Flag("state-file-configmap-name", "Name of the configmap with state file.").Default("p1-bigquery-exporter").OverrideDefaultFromEnvar("MEASUREMENT_FILE_CONFIG_MAP_NAME").String()
)

func main() {

	// parse command line parameters
	kingpin.Parse()

	// init log format from envvar ESTAFETTE_LOG_FORMAT
	foundation.InitLoggingFromEnv(foundation.NewApplicationInfo(appgroup, app, version, branch, revision, buildDate))

	// read config from yaml file
	config, err := readConfigFromFile(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed loading config from %v", *configPath)
	}

	log.Info().Interface("supportedReadings", config.SupportedReadings).Msgf("Loaded config from %v", *configPath)

	// init bigquery client
	bigqueryClient, err := NewBigQueryClient(*bigqueryProjectID, *bigqueryEnable)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed creating bigquery client")
	}

	// init bigquery table if it doesn't exist yet
	initBigqueryTable(bigqueryClient)

	// create kubernetes api client
	kubeClientConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal().Err(err)
	}
	// creates the clientset
	kubeClientset, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		log.Fatal().Err(err)
	}

	// get previous measurement
	measurementMap := readLastMeasurementFromMeasurementFile()

	log.Info().Msgf("Read from serial usb device at %v for readings from the P1 smart meter...", *p1DevicePath)
	serialConfig := &serial.Config{Name: *p1DevicePath, Baud: 115200}
	usb, err := serial.OpenPort(serialConfig)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed opening usb port at %v", *p1DevicePath)
	}
	defer usb.Close()

	hasRecordedReading := map[string]bool{}

	measurement := BigQueryMeasurement{
		Readings:   []BigQuerySmartMeterReading{},
		InsertedAt: time.Now().UTC(),
	}

	reader := bufio.NewReader(usb)
	for {
		if len(hasRecordedReading) >= len(config.SupportedReadings) {
			log.Info().Msgf("Collected %v readings, stop reading for more", len(measurement.Readings))
			break
		}

		// read from usb port
		rawLine, err := reader.ReadBytes('\x0a')
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed reading from usb port at %v", *p1DevicePath)
		}

		line := string(rawLine[:])
		log.Debug().Msg(line)

		for _, r := range config.SupportedReadings {
			if !strings.HasPrefix(line, r.Prefix) {
				continue
			}

			if len(line) < r.ValueStartIndex+r.ValueLength {
				log.Warn().Msgf("Line with length %v is too short to extract value for reading '%v'", len(line), r.Name)
				break
			}

			valueAsString := line[r.ValueStartIndex : r.ValueStartIndex+r.ValueLength]
			valueAsFloat64, err := strconv.ParseFloat(valueAsString, 64)
			if err != nil {
				log.Warn().Err(err).Msgf("Failed parsing float '%v' for reading '%v'", valueAsString, r.Name)
				break
			}

			valueAsFloat64 = valueAsFloat64 * r.ValueMultiplier
			log.Info().Msgf("%v: %v%v", r.Name, valueAsFloat64, r.Unit)

			// if difference with previous measurement is too large ( > 10 kWh ) ignore, the p1 connection probably returned an incorrect reading
			if previousValueAsFloat64, ok := measurementMap[r.Name]; ok && r.Unit == "Wh" && valueAsFloat64-previousValueAsFloat64 > 10*1000 {
				log.Warn().Msgf("Increase for reading '%v' is %v, more than the allowed 10 kWh, skipping the reading", r.Name, valueAsFloat64-previousValueAsFloat64)
				break
			}

			if _, ok := hasRecordedReading[r.Name]; !ok {
				// map to BigQuerySmartMeterReading
				measurement.Readings = append(measurement.Readings, BigQuerySmartMeterReading{
					Name:    r.Name,
					Reading: valueAsFloat64,
					Unit:    r.Unit,
				})
				hasRecordedReading[r.Name] = true
			} else {
				log.Warn().Msgf("A reading for %v has already been recorded", r.Name)
			}

			break
		}
	}

	err = bigqueryClient.InsertMeasurement(*bigqueryDataset, *bigqueryTable, measurement)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed inserting measurements into bigquery table")
	}

	writeMeasurementToConfigmap(kubeClientset, measurement)

	log.Info().Msgf("Stored %v readings, exiting...", len(measurement.Readings))
}

func initBigqueryTable(bigqueryClient BigQueryClient) {

	log.Debug().Msgf("Checking if table %v.%v.%v exists...", *bigqueryProjectID, *bigqueryDataset, *bigqueryTable)
	tableExist := bigqueryClient.CheckIfTableExists(*bigqueryDataset, *bigqueryTable)
	if !tableExist {
		log.Debug().Msgf("Creating table %v.%v.%v...", *bigqueryProjectID, *bigqueryDataset, *bigqueryTable)
		err := bigqueryClient.CreateTable(*bigqueryDataset, *bigqueryTable, BigQueryMeasurement{}, "inserted_at", true)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed creating bigquery table")
		}
	} else {
		log.Debug().Msgf("Trying to update table %v.%v.%v schema...", *bigqueryProjectID, *bigqueryDataset, *bigqueryTable)
		err := bigqueryClient.UpdateTableSchema(*bigqueryDataset, *bigqueryTable, BigQueryMeasurement{})
		if err != nil {
			log.Fatal().Err(err).Msg("Failed updating bigquery table schema")
		}
	}
}

func readConfigFromFile(configFilePath string) (config Config, err error) {
	log.Debug().Msgf("Reading %v file...", configFilePath)

	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return config, err
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, err
	}

	return
}

func readLastMeasurementFromMeasurementFile() (measurementMap map[string]float64) {

	measurementMap = map[string]float64{}

	// check if last measurement file exists in configmap
	var lastMeasurement BigQueryMeasurement
	if _, err := os.Stat(*measurementFilePath); !os.IsNotExist(err) {
		log.Info().Msgf("File %v exists, reading contents...", *measurementFilePath)

		// read state file
		data, err := ioutil.ReadFile(*measurementFilePath)
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed reading file from path %v", *measurementFilePath)
		}

		log.Info().Msgf("Unmarshalling file %v contents...", *measurementFilePath)

		// unmarshal state file
		if err := json.Unmarshal(data, &lastMeasurement); err != nil {
			log.Fatal().Err(err).Interface("data", data).Msg("Failed unmarshalling last measurement file")
		}

		for _, r := range lastMeasurement.Readings {
			measurementMap[r.Name] = r.Reading
		}
	}

	return measurementMap
}

func writeMeasurementToConfigmap(kubeClientset *kubernetes.Clientset, measurement BigQueryMeasurement) {

	// retrieve configmap
	configMap, err := kubeClientset.CoreV1().ConfigMaps(getCurrentNamespace()).Get(*measurementFileConfigMapName, metav1.GetOptions{})
	if err != nil {
		log.Error().Err(err).Msgf("Failed retrieving configmap %v", *measurementFileConfigMapName)
	}

	// marshal state to json
	measurementData, err := json.Marshal(measurement)
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	configMap.Data[filepath.Base(*measurementFilePath)] = string(measurementData)

	// update configmap to have measurement available when the application runs the next time and for other applications
	_, err = kubeClientset.CoreV1().ConfigMaps(getCurrentNamespace()).Update(configMap)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed updating configmap %v", *measurementFileConfigMapName)
	}

	log.Info().Msgf("Stored measurement in configmap %v...", *measurementFileConfigMapName)
}

func getCurrentNamespace() string {
	namespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed reading namespace")
	}

	return string(namespace)
}
