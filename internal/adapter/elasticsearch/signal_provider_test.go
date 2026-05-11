package elasticsearch

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

func TestLogProviderCollectSignalsMapsLogSignals(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"hits": {
				"hits": [
					{
						"_id": "log-1",
						"_index": "logs-app-default",
						"_source": {
							"@timestamp": "2026-03-30T14:00:00Z",
							"message": "segments-api timeout",
							"log": {
								"level": "error"
							},
							"service": {
								"name": "segments-api",
								"environment": "qa3"
							}
						}
					}
				]
			}
		}`))
	}))
	defer server.Close()

	provider := NewLogProvider(config.ElasticConfig{
		ProviderConfig: config.ProviderConfig{
			BaseURL: server.URL,
		},
		EnvironmentFields: []string{"service.environment"},
	})

	signals, err := provider.CollectSignals(context.Background(), outbound.CollectSignalsQuery{
		Services: []string{"segments-api"},
		Since:    time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC),
		Until:    time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Source != domain.SignalSourceElastic {
		t.Fatalf("expected elastic signal source, got %q", signals[0].Source)
	}
	if signals[0].Type != domain.SignalTypeLog {
		t.Fatalf("expected log signal type, got %q", signals[0].Type)
	}
	if signals[0].Severity != domain.SignalSeverityHigh {
		t.Fatalf("expected high signal severity, got %q", signals[0].Severity)
	}
	if signals[0].Summary != "segments-api timeout" {
		t.Fatalf("unexpected summary %q", signals[0].Summary)
	}
}
