package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
)

func TestEventAlertProviderFetchAlertsMapsMonitorAlertEvents(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/events/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.Header.Get("DD-API-KEY") != "api-key" {
			t.Fatalf("unexpected api key header")
		}
		if r.Header.Get("DD-APPLICATION-KEY") != "app-key" {
			t.Fatalf("unexpected application key header")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "evt-1",
					"type": "event",
					"attributes": {
						"message": "segments-api latency is too high",
						"tags": ["source:alert","env:qa3","service:segments-api"],
						"timestamp": "2026-03-27T18:58:47Z",
						"attributes": {
							"monitor_id": 105833387,
							"service": "segments-api",
							"status": "error",
							"title": "[Triggered] Service segments-api has a high response latency on env:qa3",
							"tags": ["monitor","team:lbc-data-discovery"],
							"timestamp": 1774637927000,
							"monitor": {
								"id": 105833387,
								"name": "Service segments-api has a high response latency on env:qa3",
								"query": "max(last_5m):avg:trace.http.request{env:qa3,service:segments-api} > 0.1",
								"tags": ["env:qa3","team:lbc-data-discovery","service:segments-api"],
								"type": "query alert"
							}
						}
					}
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewEventAlertProvider(config.DatadogConfig{
		BaseURL:     server.URL,
		APIKey:      "api-key",
		AppKey:      "app-key",
		MonitorTags: []string{"env:qa3"},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Date(2026, 3, 27, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Source != domain.SourceDatadog {
		t.Fatalf("expected datadog source, got %s", events[0].Source)
	}
	if events[0].Service != "segments-api" {
		t.Fatalf("expected service segments-api, got %s", events[0].Service)
	}
	if events[0].Environment != "qa3" {
		t.Fatalf("expected environment qa3, got %s", events[0].Environment)
	}
}

func TestEventAlertProviderFetchAlertsFiltersByConfiguredTags(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "evt-2",
					"type": "event",
					"attributes": {
						"message": "generatorix error rate is too high",
						"tags": ["source:alert","env:preprod","service:generatorix"],
						"timestamp": "2026-03-27T18:58:47Z",
						"attributes": {
							"monitor_id": 42,
							"service": "generatorix",
							"status": "error",
							"title": "Service generatorix has a high error rate on env:preprod",
							"timestamp": 1774637927000,
							"monitor": {
								"id": 42,
								"name": "Service generatorix has a high error rate on env:preprod",
								"tags": ["env:preprod","service:generatorix"]
							}
						}
					}
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewEventAlertProvider(config.DatadogConfig{
		BaseURL:     server.URL,
		APIKey:      "api-key",
		AppKey:      "app-key",
		MonitorTags: []string{"env:qa3"},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Date(2026, 3, 27, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestEventAlertProviderFetchAlertsRetriesOnConfiguredSiteFailure(t *testing.T) {
	t.Parallel()

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer failingServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer successServer.Close()

	provider := NewEventAlertProvider(config.DatadogConfig{
		BaseURL: failingServer.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})
	provider.baseURLs = []string{failingServer.URL, successServer.URL}

	_, err := provider.FetchAlerts(context.Background(), time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
