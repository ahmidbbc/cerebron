package handlerhttp

import (
	"context"
	"io"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"cerebron/internal/domain"
	"cerebron/internal/metrics"
	"cerebron/internal/usecase/analyzecausalhints"
	"cerebron/internal/usecase/detectincidenttrends"
	"cerebron/internal/usecase/findsimilarincidents"
	"cerebron/internal/usecase/getincidenthistory"
	"cerebron/internal/usecase/getrecentdeployments"
	"cerebron/internal/usecase/getservicedependencies"
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

type detectIncidentTrendsUseCaseStub struct {
	result domain.IncidentTrends
	err    error
}

func (s detectIncidentTrendsUseCaseStub) Run(_ context.Context, _ detectincidenttrends.Input) (domain.IncidentTrends, error) {
	if s.err != nil {
		return domain.IncidentTrends{}, s.err
	}
	return s.result, nil
}

type getServiceDependenciesUseCaseStub struct {
	result domain.ServiceDependencies
	err    error
}

func (s getServiceDependenciesUseCaseStub) Run(_ context.Context, _ getservicedependencies.Input) (domain.ServiceDependencies, error) {
	if s.err != nil {
		return domain.ServiceDependencies{}, s.err
	}
	return s.result, nil
}

type analyzeCausalHintsUseCaseStub struct {
	result domain.CausalAnalysis
	err    error
}

func (s analyzeCausalHintsUseCaseStub) Run(_ context.Context, _ analyzecausalhints.Input) (domain.CausalAnalysis, error) {
	if s.err != nil {
		return domain.CausalAnalysis{}, s.err
	}
	return s.result, nil
}

type getRecentDeploymentsUseCaseStub struct {
	result getrecentdeployments.Result
	err    error
}

func (s getRecentDeploymentsUseCaseStub) Run(_ context.Context, _ getrecentdeployments.Input) (getrecentdeployments.Result, error) {
	if s.err != nil {
		return getrecentdeployments.Result{}, s.err
	}
	return s.result, nil
}

type getIncidentHistoryUseCaseStub struct {
	result getincidenthistory.Result
	err    error
}

func (s getIncidentHistoryUseCaseStub) Run(_ context.Context, _ getincidenthistory.Input) (getincidenthistory.Result, error) {
	if s.err != nil {
		return getincidenthistory.Result{}, s.err
	}
	return s.result, nil
}
