# What does this do?

In the Netherlands so called 'smart' electricity meters are nothing more than meters that can export their readings in a digital format, called P1.

The specification of the format can be found at https://www.netbeheernederland.nl/_upload/Files/Slimme_meter_15_a727fce1f1.pdf.

Multiple other metering devices can be linked to the smart meter and export their readings through the same format; this is often used to export gas meter readings as well; and although it can also be used to link water meters in the Netherlands this is rarely done, because water is supplied by different companies than energy.

This application can be run as a cronjob to export the readings to BigQuery in a regular interval.


## Installation

To install this application using Helm run the following commands: 

```bash
helm repo add jorritsalverda https://helm.jorritsalverda.com
kubectl create namespace p1-bigquery-exporter

helm upgrade \
  p1-bigquery-exporter \
  jorritsalverda/p1-bigquery-exporter \
  --install \
  --namespace p1-bigquery-exporter \
  --set secret.gcpServiceAccountKeyfile='{abc: blabla}' \
  --wait
```

If you later on want to upgrade without specifying all values again you can use

```bash
helm upgrade \
  p1-bigquery-exporter \
  jorritsalverda/p1-bigquery-exporter \
  --install \
  --namespace p1-bigquery-exporter \
  --reuse-values \
  --set cronjob.schedule='*/1 * * * *' \
  --wait
```