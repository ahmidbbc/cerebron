package handlerhttp

import (
	"io"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"cerebron/internal/metrics"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testMetrics() *metrics.Metrics {
	return metrics.New(prometheus.NewRegistry())
}

func testGatherer() prometheus.Gatherer {
	return prometheus.NewRegistry()
}
