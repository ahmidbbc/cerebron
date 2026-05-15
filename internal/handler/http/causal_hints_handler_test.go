package handlerhttp

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/domain"
)

func TestCausalHintsHandlerReturnsOK(t *testing.T) {
	t.Parallel()

	stub := analyzeCausalHintsUseCaseStub{
		result: domain.CausalAnalysis{
			Service: "payments",
			Hints: []domain.CausalHint{
				{Rule: domain.CausalRuleDeploymentTriggered, Confidence: 0.8, Evidence: "deploy before incident"},
			},
		},
	}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(stub),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	body := bytes.NewBufferString(`{"service":"payments","model_version":"v1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/causal-hints", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCausalHintsHandlerReturnsBadRequestForInvalidBody(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/causal-hints", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCausalHintsHandlerReturnsInternalServerErrorOnUsecaseFailure(t *testing.T) {
	t.Parallel()

	stub := analyzeCausalHintsUseCaseStub{err: errors.New("causal analysis failed")}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(stub),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	body := bytes.NewBufferString(`{"service":"payments","model_version":"v1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/causal-hints", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
