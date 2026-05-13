package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/config"
)

func TestServiceDiscoveryExtractsServicesAndEnvironments(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/monitor" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("DD-API-KEY") != "api-key" {
			t.Fatalf("unexpected api key header")
		}
		if r.URL.Query().Get("group_states") != "all" {
			t.Fatalf("unexpected group_states query %s", r.URL.Query().Get("group_states"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": 1,
				"query": "avg(last_5m):...",
				"tags": ["service:catalog-api","env:prod","team:search"]
			},
			{
				"id": 2,
				"query": "avg(last_5m):...",
				"tags": ["service:catalog-api","env:preprod"]
			},
			{
				"id": 3,
				"query": "avg(last_5m):...",
				"tags": ["service:payment-api","env:prod"]
			}
		]`))
	}))
	defer server.Close()

	d := NewServiceDiscovery(config.DatadogConfig{
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	result, err := d.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.Services) != 2 {
		t.Fatalf("expected 2 services, got %d: %v", len(result.Services), result.Services)
	}
	if result.Services[0] != "catalog-api" {
		t.Fatalf("expected first service catalog-api, got %s", result.Services[0])
	}
	if result.Services[1] != "payment-api" {
		t.Fatalf("expected second service payment-api, got %s", result.Services[1])
	}

	if len(result.Environments) != 2 {
		t.Fatalf("expected 2 environments, got %d: %v", len(result.Environments), result.Environments)
	}
}

func TestServiceDiscoveryExtractsServiceFromQuery(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": 10,
				"query": "sum(last_5m):count:http.requests{service:import-api,env:prod}.as_count() > 0",
				"tags": []
			}
		]`))
	}))
	defer server.Close()

	d := NewServiceDiscovery(config.DatadogConfig{
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	result, err := d.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.Services) != 1 {
		t.Fatalf("expected 1 service from query, got %d: %v", len(result.Services), result.Services)
	}
	if result.Services[0] != "import-api" {
		t.Fatalf("expected import-api, got %s", result.Services[0])
	}
}

func TestServiceDiscoveryCapabilitiesMonitorCount(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 1, "query": "", "tags": ["service:svc-a","env:prod"]},
			{"id": 2, "query": "", "tags": ["service:svc-a","env:preprod"]},
			{"id": 3, "query": "", "tags": ["service:svc-b","env:prod"]}
		]`))
	}))
	defer server.Close()

	d := NewServiceDiscovery(config.DatadogConfig{
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	result, err := d.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(result.Capabilities))
	}

	svcA := result.Capabilities[0]
	if svcA.Service != "svc-a" {
		t.Fatalf("expected svc-a, got %s", svcA.Service)
	}
	if svcA.MonitorCount != 2 {
		t.Fatalf("expected 2 monitors for svc-a, got %d", svcA.MonitorCount)
	}
	if len(svcA.Environments) != 2 {
		t.Fatalf("expected 2 environments for svc-a, got %d", len(svcA.Environments))
	}

	svcB := result.Capabilities[1]
	if svcB.MonitorCount != 1 {
		t.Fatalf("expected 1 monitor for svc-b, got %d", svcB.MonitorCount)
	}
}

func TestServiceDiscoverySkipsMonitorsWithNoService(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 1, "query": "avg(last_5m):...", "tags": ["env:prod"]},
			{"id": 2, "query": "avg(last_5m):...", "tags": ["service:real-svc","env:prod"]}
		]`))
	}))
	defer server.Close()

	d := NewServiceDiscovery(config.DatadogConfig{
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	result, err := d.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.Services) != 1 {
		t.Fatalf("expected 1 service (monitor without service tag skipped), got %d: %v", len(result.Services), result.Services)
	}
	if result.Services[0] != "real-svc" {
		t.Fatalf("expected real-svc, got %s", result.Services[0])
	}
}

func TestServiceDiscoveryRetriesOnFallbackSite(t *testing.T) {
	t.Parallel()

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer failing.Close()

	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id": 5, "query": "", "tags": ["service:fallback-svc","env:qa"]}]`))
	}))
	defer success.Close()

	d := NewServiceDiscovery(config.DatadogConfig{
		BaseURL: failing.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})
	d.baseURLs = []string{failing.URL, success.URL}

	result, err := d.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("expected no error after fallback, got %v", err)
	}
	if len(result.Services) != 1 || result.Services[0] != "fallback-svc" {
		t.Fatalf("expected fallback-svc, got %v", result.Services)
	}
}

func TestServiceDiscoveryEmptyResponseReturnsEmptyResult(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	d := NewServiceDiscovery(config.DatadogConfig{
		BaseURL: server.URL,
		APIKey:  "api-key",
		AppKey:  "app-key",
	})

	result, err := d.DiscoverServices(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Services) != 0 {
		t.Fatalf("expected 0 services, got %d", len(result.Services))
	}
	if len(result.Environments) != 0 {
		t.Fatalf("expected 0 environments, got %d", len(result.Environments))
	}
}
