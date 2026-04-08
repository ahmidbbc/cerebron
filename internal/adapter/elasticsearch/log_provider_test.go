package elasticsearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
)

func TestLogProviderSearchLogsMapsElasticHits(t *testing.T) {
	t.Parallel()

	var requestAuthorization string
	var requestPath string
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestAuthorization = request.Header.Get("Authorization")
		requestPath = request.URL.Path

		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

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
								"level": "error",
								"logger": "http.middleware"
							},
							"service": {
								"name": "segments-api",
								"environment": "qa3"
							},
							"team": "adaptive-pricing",
							"trace": {
								"id": "trace-123"
							}
						}
					},
					{
						"_id": "log-2",
						"_index": "logs-app-default",
						"_source": {
							"@timestamp": "2026-03-30T14:01:00Z",
							"message": "background heartbeat",
							"log": {
								"level": "info"
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
			Token:   "elastic-api-key",
		},
		IndexPattern:      "logs-*/",
		EnvironmentFields: []string{"k8s-namespace", "service.environment", "environment", "env", "labels.env"},
	})

	events, err := provider.SearchLogs(context.Background(), domain.LogQuery{
		Since: time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if requestAuthorization != "ApiKey elastic-api-key" {
		t.Fatalf("expected ApiKey authorization, got %q", requestAuthorization)
	}
	if requestPath != "/logs-*/_search" {
		t.Fatalf("expected search path /logs-*/_search, got %q", requestPath)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 mapped event, got %d", len(events))
	}
	if events[0].Source != domain.SourceElasticsearch {
		t.Fatalf("expected elasticsearch event source, got %s", events[0].Source)
	}
	if events[0].Severity != domain.SeverityAlert {
		t.Fatalf("expected alert severity, got %s", events[0].Severity)
	}
	if events[0].Service != "segments-api" {
		t.Fatalf("expected service segments-api, got %q", events[0].Service)
	}
	if events[0].Environment != "qa3" {
		t.Fatalf("expected environment qa3, got %q", events[0].Environment)
	}
	if events[0].OwnerTeam != "adaptive-pricing" {
		t.Fatalf("expected owner team adaptive-pricing, got %q", events[0].OwnerTeam)
	}
	if !strings.Contains(events[0].Fingerprint, "http.middleware") {
		t.Fatalf("expected fingerprint to include logger, got %q", events[0].Fingerprint)
	}
	if requestBody["size"] != float64(100) {
		t.Fatalf("expected size 100 in request body, got %v", requestBody["size"])
	}
}

func TestLogProviderSearchLogsMapsHTTPStatusEventsEvenWhenLevelIsInfo(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"hits": {
				"hits": [
					{
						"_id": "log-409",
						"_index": "k8s-fiaas-app-morpheus-001516",
						"_source": {
							"@timestamp": "2026-04-02T17:10:07Z",
							"application": "import-ads-booking-private-api",
							"k8s-namespace": "prod-morpheus",
							"level": "info",
							"status": "409",
							"message": "quota exceeded",
							"error": "quota exceeded",
							"request-url": "/api/import-ads-booking/v1/bookings"
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
		IndexPattern:      "k8s-fiaas-app-morpheus-*",
		EnvironmentFields: []string{"k8s-namespace"},
	})

	events, err := provider.SearchLogs(context.Background(), domain.LogQuery{
		Since: time.Date(2026, 4, 2, 16, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 4, 2, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 mapped event, got %d", len(events))
	}
	if events[0].Severity != domain.SeverityWarning {
		t.Fatalf("expected warning severity from HTTP 409, got %s", events[0].Severity)
	}
	if events[0].Service != "import-ads-booking-private-api" {
		t.Fatalf("expected application to map service, got %q", events[0].Service)
	}
	if events[0].Environment != "prod-morpheus" {
		t.Fatalf("expected k8s namespace to map environment, got %q", events[0].Environment)
	}
	if events[0].StatusCode != 409 {
		t.Fatalf("expected status code 409, got %d", events[0].StatusCode)
	}
	if events[0].StatusClass != "4xx" {
		t.Fatalf("expected status class 4xx, got %q", events[0].StatusClass)
	}
	if events[0].Route != "/api/import-ads-booking/v1/bookings" {
		t.Fatalf("expected request-url to map route, got %q", events[0].Route)
	}
	if events[0].Error != "quota exceeded" {
		t.Fatalf("expected error field to map, got %q", events[0].Error)
	}
}

func TestLogProviderSearchLogsAcceptsExplicitAuthorizationScheme(t *testing.T) {
	t.Parallel()

	var requestAuthorization string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestAuthorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()

	provider := NewLogProvider(config.ElasticConfig{
		ProviderConfig: config.ProviderConfig{
			BaseURL: server.URL,
			Token:   "Bearer elastic-token",
		},
		EnvironmentFields: []string{"k8s-namespace"},
	})

	_, err := provider.SearchLogs(context.Background(), domain.LogQuery{
		Since: time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if requestAuthorization != "Bearer elastic-token" {
		t.Fatalf("expected bearer authorization to be preserved, got %q", requestAuthorization)
	}
}

func TestLogProviderSearchLogsOmitsAuthorizationHeaderWhenTokenIsEmpty(t *testing.T) {
	t.Parallel()

	var requestAuthorization string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestAuthorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()

	provider := NewLogProvider(config.ElasticConfig{
		ProviderConfig: config.ProviderConfig{
			BaseURL: server.URL,
		},
		EnvironmentFields: []string{"k8s-namespace"},
	})

	_, err := provider.SearchLogs(context.Background(), domain.LogQuery{
		Since: time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if requestAuthorization != "" {
		t.Fatalf("expected no authorization header, got %q", requestAuthorization)
	}
}

func TestLogProviderSearchLogsReturnsProviderError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, `{"error":"boom"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := NewLogProvider(config.ElasticConfig{
		ProviderConfig: config.ProviderConfig{
			BaseURL: server.URL,
			Token:   "elastic-api-key",
		},
		EnvironmentFields: []string{"k8s-namespace"},
	})

	_, err := provider.SearchLogs(context.Background(), domain.LogQuery{
		Since: time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatalf("expected provider error")
	}
}

func TestLogProviderSearchLogsPrefiltersByEnvironmentFamily(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()

	provider := NewLogProvider(config.ElasticConfig{
		ProviderConfig: config.ProviderConfig{
			BaseURL: server.URL,
		},
		IndexPattern:      "k8s-fiaas-app-identity-*",
		EnvironmentFields: []string{"k8s-namespace"},
	})

	_, err := provider.SearchLogs(context.Background(), domain.LogQuery{
		Environment: "preprod",
		Since:       time.Date(2026, 4, 3, 8, 0, 0, 0, time.UTC),
		Until:       time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	queryBody, ok := requestBody["query"].(map[string]any)
	if !ok {
		t.Fatalf("expected query body, got %v", requestBody)
	}
	boolBody, ok := queryBody["bool"].(map[string]any)
	if !ok {
		t.Fatalf("expected bool query body, got %v", queryBody)
	}
	filters, ok := boolBody["filter"].([]any)
	if !ok {
		t.Fatalf("expected filter list, got %v", boolBody["filter"])
	}
	if len(filters) != 2 {
		t.Fatalf("expected time and environment filters, got %v", filters)
	}

	environmentFilter, ok := filters[1].(map[string]any)
	if !ok {
		t.Fatalf("expected environment filter map, got %v", filters[1])
	}
	environmentBool, ok := environmentFilter["bool"].(map[string]any)
	if !ok {
		t.Fatalf("expected bool environment filter, got %v", environmentFilter)
	}
	if environmentBool["minimum_should_match"] != float64(1) {
		t.Fatalf("expected minimum_should_match=1, got %v", environmentBool["minimum_should_match"])
	}
	shouldClauses, ok := environmentBool["should"].([]any)
	if !ok {
		t.Fatalf("expected should clauses, got %v", environmentBool["should"])
	}
	if len(shouldClauses) != 2 {
		t.Fatalf("expected 2 prefix clauses for one environment field, got %d", len(shouldClauses))
	}

	firstClause, ok := shouldClauses[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first clause map, got %v", shouldClauses[0])
	}
	prefixClause, ok := firstClause["prefix"].(map[string]any)
	if !ok {
		t.Fatalf("expected prefix clause, got %v", firstClause)
	}
	if prefixClause["k8s-namespace"] != "preprod" {
		t.Fatalf("expected k8s-namespace prefix preprod, got %v", prefixClause["k8s-namespace"])
	}
}

func TestLogProviderSearchLogsAddsServiceAndTermsFilters(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()

	provider := NewLogProvider(config.ElasticConfig{
		ProviderConfig: config.ProviderConfig{
			BaseURL: server.URL,
		},
		IndexPattern:      "k8s-fiaas-app-identity-*",
		EnvironmentFields: []string{"k8s-namespace"},
	})

	_, err := provider.SearchLogs(context.Background(), domain.LogQuery{
		Environment: "preprod",
		Service:     "presence-api",
		Terms:       []string{"illegal base64 data at input byte 0"},
		Since:       time.Date(2026, 4, 3, 8, 0, 0, 0, time.UTC),
		Until:       time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	queryBody, ok := requestBody["query"].(map[string]any)
	if !ok {
		t.Fatalf("expected query body, got %v", requestBody)
	}
	boolBody, ok := queryBody["bool"].(map[string]any)
	if !ok {
		t.Fatalf("expected bool query body, got %v", queryBody)
	}
	filters, ok := boolBody["filter"].([]any)
	if !ok {
		t.Fatalf("expected filter list, got %v", boolBody["filter"])
	}
	if len(filters) != 4 {
		t.Fatalf("expected time, environment, service and terms filters, got %v", filters)
	}
}
