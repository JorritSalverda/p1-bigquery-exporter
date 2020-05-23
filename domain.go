package main

import (
	"time"
)

type Config struct {
	SupportedReadings []Reading `yaml:"supportedReadings"`
}

type Reading struct {
	Prefix          string  `yaml:"prefix"`
	Name            string  `yaml:"name"`
	Unit            string  `yaml:"unit"`
	ValueMultiplier float64 `yaml:"valueMultiplier"`
	ValueStartIndex int     `yaml:"valueStartIndex"`
	ValueLength     int     `yaml:"valueLength"`
}

type BigQueryMeasurement struct {
	Readings   []BigQuerySmartMeterReading `bigquery:"readings"`
	InsertedAt time.Time                   `bigquery:"inserted_at"`
}

type BigQuerySmartMeterReading struct {
	Name    string  `bigquery:"name"`
	Reading float64 `bigquery:"reading"`
	Unit    string  `bigquery:"unit"`
}
