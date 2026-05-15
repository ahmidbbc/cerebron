package handlerhttp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/analyzeincident"
)

type analyzeIncidentUseCaseStub struct {
	result domain.IncidentAnalysis
	err    error
}

func (s analyzeIncidentUseCaseStub) Run(_ context.Context, _ analyzeincident.Input) (domain.IncidentAnalysis, error) {
	if s.err != nil {
		return domain.IncidentAnalysis{}, s.err
	}

	return s.result, nil
}

func TestIncidentHandlerAnalyzeReturnsOK(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{
			result: domain.IncidentAnalysis{
				Service:      "catalog-api",
				TimeRange:    "2026-04-30T10:00:00Z/2026-04-30T11:00:00Z",
				ModelVersion: domain.IncidentAnalysisModelVersion,
				Summary:      "No incident signals detected for service catalog-api.",
				Confidence:   0.0,
			},
		}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/analyze", bytes.NewBufferString(`{"service":"catalog-api","time_range":"1h"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"service":"catalog-api"`)) {
		t.Fatalf("expected service in response body, got %s", recorder.Body.String())
	}
}

func TestIncidentHandlerAnalyzeReturnsBadRequestForInvalidTimeRange(t *testing.T) {
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

	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/analyze", bytes.NewBufferString(`{"service":"catalog-api","time_range":"invalid"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestIncidentHandlerAnalyzeReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{err: errors.New("analysis failed")}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/analyze", bytes.NewBufferString(`{"service":"catalog-api"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
}

func TestIncidentHandlerAnalyzeAcceptsEmptyTimeRange(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{
			result: domain.IncidentAnalysis{Service: "catalog-api"},
		}),
		NewSimilarIncidentsHandler(findSimilarIncidentsUseCaseStub{}),
		NewTrendsHandler(detectIncidentTrendsUseCaseStub{}),
		NewServiceDependenciesHandler(getServiceDependenciesUseCaseStub{}),
		NewCausalHintsHandler(analyzeCausalHintsUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}, findSimilarIncidentsUseCaseStub{}, detectIncidentTrendsUseCaseStub{}, getServiceDependenciesUseCaseStub{}, analyzeCausalHintsUseCaseStub{}, getRecentDeploymentsUseCaseStub{}, getIncidentHistoryUseCaseStub{}, testLogger(), testMetrics()),
		testLogger(), testMetrics(), testGatherer(),
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/analyze", bytes.NewBufferString(`{"service":"catalog-api"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}
