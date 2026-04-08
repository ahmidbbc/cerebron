package handlerhttp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/usecase/monitor"
)

type monitoringUseCaseStub struct {
	result monitor.Result
	err    error
}

func (s monitoringUseCaseStub) Run(context.Context, monitor.Input) (monitor.Result, error) {
	if s.err != nil {
		return monitor.Result{}, s.err
	}

	return s.result, nil
}

func TestMonitoringHandlerRunReturnsOK(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewMonitoringHandler(monitoringUseCaseStub{
			result: monitor.Result{
				AlertEvents:          1,
				LogEvents:            2,
				CollectedEvents:      3,
				CorrelatedEvents:     1,
				ObservedEnvironments: []string{"qa"},
				Reportable:           true,
				Score:                75,
			},
		}),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/signals/run", bytes.NewBufferString(`{"since":"15m","environments":["qa"]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"correlated_events":1`)) {
		t.Fatalf("expected correlated events in response body, got %s", recorder.Body.String())
	}
}

func TestMonitoringHandlerRunReturnsDebugDataWhenRequested(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewMonitoringHandler(monitoringUseCaseStub{
			result: monitor.Result{
				ObservedEnvironments: []string{"preprod"},
				Debug: []monitor.ProviderDebug{{
					Provider:           "datadog",
					MonitorsFetched:    4,
					CandidateEvents:    2,
					ReturnedByProvider: 1,
				}},
				DebugEvents: []monitor.DebugEvent{{
					Source:      "datadog",
					Service:     "presence-api",
					Environment: "preprod",
				}},
				DebugCorrelations: []monitor.DebugCorrelation{{
					Alert: monitor.DebugEvent{
						Source:  "datadog",
						Service: "presence-api",
					},
					Log: monitor.DebugEvent{
						Source:  "elasticsearch",
						Service: "presence-api",
					},
					Score:   8,
					Reasons: []string{"same_service", "same_error_signature"},
				}},
			},
		}),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/signals/run", bytes.NewBufferString(`{"since":"15m","environments":["preprod"],"debug":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"provider":"datadog"`)) {
		t.Fatalf("expected debug provider in response body, got %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"debug_events"`)) {
		t.Fatalf("expected debug events in response body, got %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"debug_correlations"`)) {
		t.Fatalf("expected debug correlations in response body, got %s", recorder.Body.String())
	}
}

func TestMonitoringHandlerRunReturnsBadRequestForInvalidDuration(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewMonitoringHandler(monitoringUseCaseStub{}),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/signals/run", bytes.NewBufferString(`{"since":"invalid"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestMonitoringHandlerRunReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewMonitoringHandler(monitoringUseCaseStub{err: errors.New("boom")}),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/signals/run", bytes.NewBufferString(`{"since":"15m"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
}

func TestMonitoringHandlerRunReturnsErrorDetailsInDebugMode(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewMonitoringHandler(monitoringUseCaseStub{err: errors.New("fetch alerts from datadog: unexpected status code 403")}),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/signals/run", bytes.NewBufferString(`{"since":"15m","debug":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"details":"fetch alerts from datadog: unexpected status code 403"`)) {
		t.Fatalf("expected error details in response body, got %s", recorder.Body.String())
	}
}
