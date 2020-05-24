# What does this do?

In the Netherlands so called 'smart' electricity meters are nothing more than meters that can export their readings in a digital format, called P1.

The specification of the format can be found at https://www.netbeheernederland.nl/_upload/Files/Slimme_meter_15_a727fce1f1.pdf.

Multiple other metering devices can be linked to the smart meter and export their readings through the same format; this is often used to export gas meter readings as well; and although it can also be used to link water meters in the Netherlands this is rarely done, because water is supplied by different companies than energy.

This application can be run as a cronjob to export the readings to BigQuery in a regular interval.

# How to run

Run this once for creating the secret with GCP service account keyfile

```bash
curl -s https://raw.githubusercontent.com/JorritSalverda/p1-bigquery-exporter/master/k8s/secret.yaml | GCP_SERVICE_ACCOUNT_KEYFILE='<base64 encoded service account keyfile>' envsubst \$GCP_SERVICE_ACCOUNT_KEYFILE | kubectl apply -f -
```

The service account keyfile can include newlines, since it's mounted as a file; so encode it using

```bash
cat keyfile.json | base64
```

In order to configure the application run

```bash
curl -s https://raw.githubusercontent.com/JorritSalverda/p1-bigquery-exporter/master/k8s/configmap.yaml | P1_DEVICE_PATH='/dev/ttyUSB0' BQ_ENABLE='true' BQ_PROJECT_ID='gcp-project-id' BQ_DATASET='my-dataset' BQ_TABLE='my-table' envsubst \$P1_DEVICE_PATH,\$BQ_ENABLE,\$BQ_PROJECT_ID,\$BQ_DATASET,\$BQ_TABLE | kubectl apply -f -
```

And for deploying (a new version of) the application run

```bash
curl -s https://raw.githubusercontent.com/JorritSalverda/p1-bigquery-exporter/master/k8s/cronjob.yaml | SCHEDULE='*/5 * * * *' P1_DEVICE_PATH='/dev/ttyUSB0' CONTAINER_TAG='0.1.5' envsubst \$SCHEDULE,\$P1_DEVICE_PATH,\$CONTAINER_TAG | kubectl apply -f -
```
