package main

import (
	"bufio"
	"io/ioutil"
	"runtime"
	"strconv"
	"strings"

	"github.com/alecthomas/kingpin"
	foundation "github.com/estafette/estafette-foundation"
	"github.com/rs/zerolog/log"
	"github.com/tarm/serial"
	"gopkg.in/yaml.v2"
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

	configPath = kingpin.Flag("config-path", "Path to the config.yaml file").Default("/configs/config.yaml").OverrideDefaultFromEnvar("CONFIG_PATH").String()
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

	log.Info().Msgf("Read from serial usb device at %v for readings from the P1 smart meter...", *p1DevicePath)
	serialConfig := &serial.Config{Name: *p1DevicePath, Baud: 115200}
	usb, err := serial.OpenPort(serialConfig)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed opening usb port at %v", *p1DevicePath)
	}
	defer usb.Close()

	reader := bufio.NewReader(usb)
	for {
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

			break
		}
	}

	// measurements := []BigQueryMeasurement{
	// 	BigQueryMeasurement{},
	// }

	// err = bigqueryClient.InsertMeasurements(*bigqueryDataset, *bigqueryTable, measurements)
	// if err != nil {
	// 	log.Fatal().Err(err).Msg("Failed inserting measurements into bigquery table")
	// }
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
