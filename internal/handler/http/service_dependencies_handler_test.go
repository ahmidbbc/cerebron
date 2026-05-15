package handlerhttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/domain"
)

func TestServiceDependenciesHandlerReturnsOK(t *testing.T) {
	t.Parallel()

	stub := getServiceDependenciesUseCaseStub{
		result: domain.ServiceDependencies{
			Service:     "api",
			Upstreams:   []string{"web"},
			Downstreams: []string{"db"},
			BlastRadius: []string{"web"},
			AllEdges:    []domain.DependencyEdge{{Source: "web", Target: "api"}, {Source: "api", Target: "db"}},
		},
	}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(stub),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/dependencies?service=api", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestServiceDependenciesHandlerReturnsInternalServerErrorOnUsecaseFailure(t *testing.T) {
	t.Parallel()

	stub := getServiceDependenciesUseCaseStub{err: errors.New("graph unavailable")}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(stub),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/dependencies?service=api", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
