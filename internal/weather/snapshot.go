// Package weather provides the weather client and snapshot types.
package weather

import "time"

// Snapshot holds the weather data for a city at a specific point in time.
//
// Source is a runtime-only field set by handlers before the response is sent.
// It is never persisted to Redis (the Redis value never contains "source").
// Consumers can expect one of three values: "cache", "origin", or "bypass".
type Snapshot struct {
	City         string    `json:"city"`
	DisplayName  string    `json:"display_name"`
	TemperatureC float64   `json:"temperature_c"`
	Condition    string    `json:"condition"`
	WeatherCode  int       `json:"weather_code"`
	FetchedAt    time.Time `json:"fetched_at"`
	Source       string    `json:"source,omitempty"`
}
