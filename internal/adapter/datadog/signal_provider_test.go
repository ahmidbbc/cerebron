package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

func TestAlertProviderCollectSignalsMapsMetricSignalsAndFiltersService(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	signals, err := provider.CollectSignals(context.Background(), outbound.CollectSignalsQuery{
		Services: []string{"catalog-api"},
		Since:   time.Unix(1774610000, 0).UTC(),
		Until:   time.Unix(1774613000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Source != domain.SignalSourceDatadog {
		t.Fatalf("expected datadog signal source, got %q", signals[0].Source)
	}
	if signals[0].Type != domain.SignalTypeMetric {
		t.Fatalf("expected metric signal type, got %q", signals[0].Type)
	}
	if signals[0].Severity != domain.SignalSeverityHigh {
		t.Fatalf("expected high signal severity, got %q", signals[0].Severity)
	}
	if signals[0].Summary != "Catalog latency" {
		t.Fatalf("expected signal summary Catalog latency, got %q", signals[0].Summary)
	}

	noSignals, err := provider.CollectSignals(context.Background(), outbound.CollectSignalsQuery{
		Services: []string{"billing-api"},
		Since:   time.Unix(1774610000, 0).UTC(),
		Until:   time.Unix(1774613000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(noSignals) != 0 {
		t.Fatalf("expected no signals for unmatched service, got %d", len(noSignals))
	}
}

func TestEventAlertProviderCollectSignalsMapsMetricSignals(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
							"timestamp": 1774637927000,
							"monitor": {
								"id": 105833387,
								"name": "Service segments-api has a high response latency on env:qa3",
								"query": "max(last_5m):avg:trace.http.request{env:qa3,service:segments-api} > 0.1",
								"tags": ["env:qa3","service:segments-api"],
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
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	signals, err := provider.CollectSignals(context.Background(), outbound.CollectSignalsQuery{
		Services: []string{"segments-api"},
		Since:   time.Date(2026, 3, 27, 18, 0, 0, 0, time.UTC),
		Until:   time.Date(2026, 3, 27, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Type != domain.SignalTypeMetric {
		t.Fatalf("expected metric signal type, got %q", signals[0].Type)
	}
	if signals[0].Severity != domain.SignalSeverityHigh {
		t.Fatalf("expected high signal severity, got %q", signals[0].Severity)
	}
	if signals[0].Summary != "[Triggered] Service segments-api has a high response latency on env:qa3" {
		t.Fatalf("unexpected summary %q", signals[0].Summary)
	}
}

func TestErrorTrackingProviderCollectSignalsMapsLogSignals(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/error-tracking/issues/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id": "issue-1",
						"type": "error_tracking_search_result"
					}
				],
				"included": [
					{
						"id": "issue-1",
						"type": "issue",
						"attributes": {
							"error_message": "illegal base64 data at input byte 0",
							"error_type": "base64.CorruptInputError",
							"first_seen": 1775213899243,
							"last_seen": 1775213954616,
							"service": "presence-api",
							"state": "OPEN"
						}
					}
				]
			}`))
		case "/api/v2/spans/events/search":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewErrorTrackingProvider(config.DatadogConfig{
		APIKey: "api-key",
		AppKey: "app-key",
		ErrorTracking: config.DatadogErrorTrackingConfig{
			BaseURL: server.URL,
			Query:   "service:presence-api env:preprod",
			Track:   "trace",
		},
	})

	signals, err := provider.CollectSignals(context.Background(), outbound.CollectSignalsQuery{
		Services: []string{"presence-api"},
		Since:   time.Unix(1775210000, 0).UTC(),
		Until:   time.Unix(1775215000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Type != domain.SignalTypeLog {
		t.Fatalf("expected log signal type, got %q", signals[0].Type)
	}
	if signals[0].Severity != domain.SignalSeverityHigh {
		t.Fatalf("expected high signal severity, got %q", signals[0].Severity)
	}
	if signals[0].Summary != "illegal base64 data at input byte 0" {
		t.Fatalf("unexpected summary %q", signals[0].Summary)
	}
}
