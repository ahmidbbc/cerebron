package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Cerebron Prometheus metrics.
type Metrics struct {
	// HTTP
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestsDuration *prometheus.HistogramVec

	// MCP
	MCPRequestsTotal    *prometheus.CounterVec
	MCPRequestsDuration *prometheus.HistogramVec

	// Providers
	ProviderLatency  *prometheus.HistogramVec
	ProviderFailures *prometheus.CounterVec

	// Analysis output
	SignalsCollected *prometheus.HistogramVec
	GroupsCreated   prometheus.Histogram
	ConfidenceScore prometheus.Histogram
}

// Recorder implements analyzeincident.MetricsRecorder backed by Metrics.
type Recorder struct{ m *Metrics }

func NewRecorder(m *Metrics) *Recorder { return &Recorder{m: m} }

func (r *Recorder) RecordProviderLatency(provider string, d time.Duration) {
	r.m.ProviderLatency.WithLabelValues(provider).Observe(d.Seconds())
}

func (r *Recorder) RecordProviderFailure(provider string) {
	r.m.ProviderFailures.WithLabelValues(provider).Inc()
}

func (r *Recorder) RecordSignalsCollected(provider string, count int) {
	r.m.SignalsCollected.WithLabelValues(provider).Observe(float64(count))
}

func (r *Recorder) RecordAnalysisResult(groups int, confidence float64) {
	r.m.GroupsCreated.Observe(float64(groups))
	r.m.ConfidenceScore.Observe(confidence)
}

func New(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		HTTPRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "cerebron_http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "route", "status"}),

		HTTPRequestsDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cerebron_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),

		MCPRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "cerebron_mcp_requests_total",
			Help: "Total number of MCP tool invocations.",
		}, []string{"tool", "status"}),

		MCPRequestsDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cerebron_mcp_request_duration_seconds",
			Help:    "Duration of MCP tool invocations in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"tool"}),

		ProviderLatency: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cerebron_provider_latency_seconds",
			Help:    "Provider CollectSignals latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider"}),

		ProviderFailures: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "cerebron_provider_failures_total",
			Help: "Total number of provider failures.",
		}, []string{"provider"}),

		SignalsCollected: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cerebron_signals_collected",
			Help:    "Number of signals collected per analysis.",
			Buckets: []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000},
		}, []string{"provider"}),

		GroupsCreated: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "cerebron_groups_created",
			Help:    "Number of signal groups created per analysis.",
			Buckets: []float64{0, 1, 2, 5, 10, 20, 50},
		}),

		ConfidenceScore: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "cerebron_confidence_score",
			Help:    "Distribution of confidence scores.",
			Buckets: []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		}),
	}
}
