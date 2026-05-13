// Package observability holds Prometheus metrics + OTel tracing setup.
package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics groups the Prometheus collectors itw-crud exposes.
type Metrics struct {
	registry             *prometheus.Registry
	RequestsTotal        *prometheus.CounterVec
	RequestDuration      *prometheus.HistogramVec
	PGQueriesTotal       *prometheus.CounterVec
	PGQueryDuration      *prometheus.HistogramVec
	OrphansCount         prometheus.Gauge
	ConcurrencyConflicts prometheus.Counter
}

// NewMetrics constructs all collectors on a dedicated registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "itw_crud_http_requests_total",
			Help: "Total HTTP requests handled by itw-crud.",
		}, []string{"method", "endpoint", "status"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "itw_crud_http_request_duration_seconds",
			Help:    "HTTP request latency, seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"method", "endpoint"}),
		PGQueriesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "itw_crud_pg_queries_total",
			Help: "Total PG queries executed.",
		}, []string{"op"}),
		PGQueryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "itw_crud_pg_query_duration_seconds",
			Help:    "PG query latency, seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		}, []string{"op"}),
		OrphansCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "itw_crud_orphans_count",
			Help: "Current number of orphan articles (pending payload but not translated).",
		}),
		ConcurrencyConflicts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "itw_crud_concurrency_conflicts_total",
			Help: "Total 412 Precondition Failed responses (ETag mismatches).",
		}),
	}
	reg.MustRegister(
		m.RequestsTotal, m.RequestDuration,
		m.PGQueriesTotal, m.PGQueryDuration,
		m.OrphansCount, m.ConcurrencyConflicts,
	)
	return m
}

// Handler returns the http.Handler that exposes the /metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
