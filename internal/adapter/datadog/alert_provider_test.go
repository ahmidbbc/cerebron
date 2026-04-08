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

func TestAlertProviderFetchAlertsMapsTriggeredGroups(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/monitor" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("DD-API-KEY") != "api-key" {
			t.Fatalf("unexpected api key header")
		}
		if r.Header.Get("DD-APPLICATION-KEY") != "app-key" {
			t.Fatalf("unexpected application key header")
		}
		if r.URL.Query().Get("group_states") != "alert,warn,no data" {
			t.Fatalf("unexpected group_states query %s", r.URL.Query().Get("group_states"))
		}
		if r.URL.Query().Get("monitor_tags") != "service:catalog,env:preprod" {
			t.Fatalf("unexpected monitor_tags query %s", r.URL.Query().Get("monitor_tags"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": 42,
				"name": "Catalog latency",
				"type": "metric alert",
				"query": "avg(last_5m):...",
				"tags": ["service:catalog-api","env:preprod","team:search"],
				"state": {
					"groups": {
						"service:catalog-api": {
							"status": "Alert",
							"last_triggered_ts": 1774612680
						}
					}
				}
			}
		]`))
	}))
	defer server.Close()

	provider := NewAlertProvider(config.DatadogConfig{
		BaseURL:     server.URL,
		APIKey:      "api-key",
		AppKey:      "app-key",
		MonitorTags: []string{"service:catalog", "env:preprod"},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1774610000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Source != domain.SourceDatadog {
		t.Fatalf("expected datadog source, got %s", events[0].Source)
	}
	if events[0].Severity != domain.SeverityAlert {
		t.Fatalf("expected alert severity, got %s", events[0].Severity)
	}
	if events[0].Service != "catalog-api" {
		t.Fatalf("expected service catalog-api, got %s", events[0].Service)
	}
}

func TestAlertProviderFetchAlertsUsesFallbackStateWhenNoGroups(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": 7,
				"name": "Indexer no data",
				"type": "query alert",
				"query": "avg(last_5m):...",
				"tags": ["service:indexer","env:qa","team:data"],
				"overall_state": "No Data",
				"overall_state_modified": "2026-03-27T10:00:00Z",
				"state": {}
			}
		]`))
	}))
	defer server.Close()

	provider := NewAlertProvider(config.DatadogConfig{
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	events, err := provider.FetchAlerts(context.Background(), time.Date(2026, 3, 27, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Severity != domain.SeverityWarning {
		t.Fatalf("expected warning severity, got %s", events[0].Severity)
	}
}

func TestAlertProviderFetchAlertsRetriesOnConfiguredSiteFailure(t *testing.T) {
	t.Parallel()

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer failingServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": 99,
				"name": "Segments latency",
				"type": "query alert",
				"query": "max(last_5m):...",
				"tags": ["service:segments-api","env:qa3","team:data"],
				"state": {
					"groups": {
						"*": {
							"status": "Alert",
							"last_triggered_ts": 1774612680
						}
					}
				}
			}
		]`))
	}))
	defer successServer.Close()

	provider := NewAlertProvider(config.DatadogConfig{
		BaseURL: failingServer.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})
	provider.baseURLs = []string{failingServer.URL, successServer.URL}

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1774610000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Service != "segments-api" {
		t.Fatalf("expected service segments-api, got %s", events[0].Service)
	}
}

func TestAlertProviderFetchAlertsDoesNotFallbackOnServerError(t *testing.T) {
	t.Parallel()

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatalf("did not expect fallback server to be called")
	}))
	defer successServer.Close()

	provider := NewAlertProvider(config.DatadogConfig{
		BaseURL: failingServer.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})
	provider.baseURLs = []string{failingServer.URL, successServer.URL}

	_, err := provider.FetchAlerts(context.Background(), time.Unix(1774610000, 0).UTC())
	if err == nil {
		t.Fatalf("expected provider error")
	}
}

func TestAlertProviderFetchAlertsExtractsCorrelationMetadataFromQuery(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": 120,
				"name": "Consume error on http.response.status_code:409",
				"type": "query alert",
				"query": "sum(last_30m):default_zero(count:http.server.request.duration {service:import-ads-booking-private-api,env:prod,http.route:/api/import-ads-booking/v1/bookings/consume,http.response.status_code:409}.as_count()) > 0",
				"tags": ["team:morpheus"],
				"state": {
					"groups": {
						"*": {
							"status": "Alert",
							"last_triggered_ts": 1775236200
						}
					}
				}
			}
		]`))
	}))
	defer server.Close()

	provider := NewAlertProvider(config.DatadogConfig{
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1775230000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Service != "import-ads-booking-private-api" {
		t.Fatalf("expected service from query, got %q", events[0].Service)
	}
	if events[0].Environment != "prod" {
		t.Fatalf("expected environment from query, got %q", events[0].Environment)
	}
	if events[0].StatusCode != 409 {
		t.Fatalf("expected status code 409, got %d", events[0].StatusCode)
	}
	if events[0].StatusClass != "4xx" {
		t.Fatalf("expected status class 4xx, got %q", events[0].StatusClass)
	}
	if events[0].Route != "/api/import-ads-booking/v1/bookings/consume" {
		t.Fatalf("expected route from query, got %q", events[0].Route)
	}
}
