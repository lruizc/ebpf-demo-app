package server

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the custom Prometheus collectors for the weather service.
type Metrics struct {
	RequestsTotal      *prometheus.CounterVec
	RequestDuration    *prometheus.HistogramVec
	OriginRequestTotal *prometheus.CounterVec
	CacheOpsTotal      *prometheus.CounterVec
	registry           *prometheus.Registry
}

// NewMetrics creates and registers all custom Prometheus metrics on a dedicated registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "weather_requests_total",
			Help: "Total weather requests by city, source and HTTP status.",
		}, []string{"city", "source", "status"}),

		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "weather_request_duration_seconds",
			Help:    "Request latency by city and source.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8},
		}, []string{"city", "source"}),

		OriginRequestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "weather_origin_requests_total",
			Help: "Total external Open-Meteo calls by city and outcome.",
		}, []string{"city", "outcome"}),

		CacheOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "weather_cache_operations_total",
			Help: "Cache get/set operations by op and outcome.",
		}, []string{"op", "outcome"}),

		registry: reg,
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.OriginRequestTotal,
		m.CacheOpsTotal,
	)
	return m
}

// Handler returns the Prometheus HTTP handler for /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
