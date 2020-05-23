# How to run

Run this once for creating the secret with GCP service account keyfile

```bash
curl https://raw.githubusercontent.com/JorritSalverda/p1-bigquery-exporter/master/k8s/secret.yaml | GCP_SERVICE_ACCOUNT_KEYFILE='<base64 encoded service account keyfile>' envsubst \$GCP_SERVICE_ACCOUNT_KEYFILE | kubectl apply -f -
```

The service account keyfile can include newlines, since it's mounted as a file; so encode it using

```bash
cat keyfile.json | base64
```

In order to configure the application run

```bash
curl https://raw.githubusercontent.com/JorritSalverda/p1-bigquery-exporter/master/k8s/configmap.yaml | P1_DEVICE_PATH='/dev/ttyUSB0' BQ_ENABLE='true' BQ_PROJECT_ID='gcp-project-id' BQ_DATASET='my-dataset' BQ_TABLE='my-table' envsubst \$P1_DEVICE_PATH,\$BQ_ENABLE,\$BQ_PROJECT_ID,\$BQ_DATASET,\$BQ_TABLE | kubectl apply -f -
```

And for deploying (a new version of) the application run

```bash
curl https://raw.githubusercontent.com/JorritSalverda/tp-link-hs110-bigquery-exporter/master/k8s/deployment.yaml | P1_DEVICE_PATH='/dev/ttyUSB0' CONTAINER_TAG='0.1.13' envsubst \$P1_DEVICE_PATH,\$CONTAINER_TAG | kubectl apply -f -
```
