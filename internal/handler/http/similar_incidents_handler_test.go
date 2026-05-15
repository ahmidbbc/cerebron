package handlerhttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/findsimilarincidents"
)

var errSentinel = errors.New("usecase failure")

func TestSimilarIncidentsHandlerReturnsBadRequestWhenNeitherParamProvided(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/similar", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSimilarIncidentsHandlerReturnsBadRequestForInvalidLimit(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	for _, bad := range []string{"abc", "0", "-1"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/similar?service=svc&limit="+bad, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("limit=%q: expected 400, got %d", bad, rec.Code)
		}
	}
}

func TestSimilarIncidentsHandlerReturnsOKForValidRequest(t *testing.T) {
	t.Parallel()

	stub := findSimilarIncidentsUseCaseStub{
		result: findsimilarincidents.Result{
			Related: []domain.StoredIncident{},
		},
	}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(stub),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/similar?service=catalog-api", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSimilarIncidentsHandlerReturnsInternalServerErrorOnUsecaseFailure(t *testing.T) {
	t.Parallel()

	stub := findSimilarIncidentsUseCaseStub{err: errSentinel}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(stub),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/similar?service=catalog-api", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
