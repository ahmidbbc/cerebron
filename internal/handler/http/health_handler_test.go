package handlerhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type healthUseCaseStub struct {
	livenessErr  error
	readinessErr error
}

func (s healthUseCaseStub) Liveness(context.Context) error {
	return s.livenessErr
}

func (s healthUseCaseStub) Readiness(context.Context) error {
	return s.readinessErr
}

func TestHealthHandlerLivenessReturnsOK(t *testing.T) {
	t.Parallel()

	router := NewRouter(NewHealthHandler(healthUseCaseStub{}), NewIncidentHandler(analyzeIncidentUseCaseStub{}), NewMCPHandler(analyzeIncidentUseCaseStub{}, testLogger(), testMetrics()), testLogger(), testMetrics(), testGatherer())
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}

func TestHealthHandlerReadinessReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	router := NewRouter(NewHealthHandler(healthUseCaseStub{readinessErr: errors.New("not ready")}), NewIncidentHandler(analyzeIncidentUseCaseStub{}), NewMCPHandler(analyzeIncidentUseCaseStub{}, testLogger(), testMetrics()), testLogger(), testMetrics(), testGatherer())
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}
