package handlerhttp

import (
	"context"
	"io"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"cerebron/internal/metrics"
	"cerebron/internal/usecase/findsimilarincidents"
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

type findSimilarIncidentsUseCaseStub struct {
	result findsimilarincidents.Result
	err    error
}

func (s findSimilarIncidentsUseCaseStub) Run(_ context.Context, _ findsimilarincidents.Input) (findsimilarincidents.Result, error) {
	if s.err != nil {
		return findsimilarincidents.Result{}, s.err
	}
	return s.result, nil
}
