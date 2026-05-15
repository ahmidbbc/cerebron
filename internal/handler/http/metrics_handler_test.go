package handlerhttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"cerebron/internal/metrics"
)

func TestMetricsEndpointExposesCerebronMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, testLogger(), m),
		testLogger(), m, reg,
	)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("cerebron_")) {
		t.Fatalf("expected cerebron_ metrics in /metrics output, got: %s", rec.Body.String())
	}
}
