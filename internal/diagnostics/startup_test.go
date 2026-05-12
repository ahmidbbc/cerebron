package diagnostics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/config"
	"cerebron/internal/diagnostics"
)

func TestRunStartupChecks_AllDisabled(t *testing.T) {
	cfg := config.Config{}
	report := diagnostics.RunStartupChecks(context.Background(), cfg, nopLogger())

	for _, p := range report.Providers {
		if p.Enabled {
			t.Errorf("provider %s should be disabled", p.Name)
		}
		if p.OK {
			t.Errorf("provider %s should not be OK when disabled", p.Name)
		}
	}
}

func TestRunStartupChecks_DatadogOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/validate" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"valid": true})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := config.Config{
		Datadog: config.DatadogConfig{
			BaseURL: srv.URL,
			APIKey:  "test-api-key",
			AppKey:  "test-app-key",
			Enabled: true,
		},
	}
	report := diagnostics.RunStartupChecks(context.Background(), cfg, nopLogger())

	monitors := findProvider(report, "datadog/monitors")
	if monitors == nil {
		t.Fatal("datadog/monitors provider missing from report")
	}
	if !monitors.OK {
		t.Errorf("expected datadog/monitors OK, got err: %v", monitors.Err)
	}
}

func TestRunStartupChecks_DatadogAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	cfg := config.Config{
		Datadog: config.DatadogConfig{
			BaseURL: srv.URL,
			APIKey:  "bad-key",
			AppKey:  "bad-key",
			Enabled: true,
		},
	}
	report := diagnostics.RunStartupChecks(context.Background(), cfg, nopLogger())

	monitors := findProvider(report, "datadog/monitors")
	if monitors == nil {
		t.Fatal("datadog/monitors provider missing from report")
	}
	if monitors.OK {
		t.Error("expected datadog/monitors to fail on auth error")
	}
	if monitors.Err == nil {
		t.Error("expected non-nil error")
	}
}

func TestRunStartupChecks_ElasticsearchOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_cluster/health":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "green"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cfg := config.Config{
		Elastic: config.ElasticConfig{
			ProviderConfig: config.ProviderConfig{
				BaseURL: srv.URL,
				Enabled: true,
			},
			IndexPattern: "*",
		},
	}
	report := diagnostics.RunStartupChecks(context.Background(), cfg, nopLogger())

	es := findProvider(report, "elasticsearch")
	if es == nil {
		t.Fatal("elasticsearch provider missing from report")
	}
	if !es.OK {
		t.Errorf("expected elasticsearch OK, got err: %v", es.Err)
	}
}

func TestRunStartupChecks_ElasticsearchClusterRed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "red"})
	}))
	defer srv.Close()

	cfg := config.Config{
		Elastic: config.ElasticConfig{
			ProviderConfig: config.ProviderConfig{
				BaseURL: srv.URL,
				Enabled: true,
			},
		},
	}
	report := diagnostics.RunStartupChecks(context.Background(), cfg, nopLogger())

	es := findProvider(report, "elasticsearch")
	if es == nil {
		t.Fatal("elasticsearch provider missing from report")
	}
	if es.OK {
		t.Error("expected elasticsearch to fail when cluster is red")
	}
}

func TestRunStartupChecks_ElasticsearchIndexNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_cluster/health" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "green"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := config.Config{
		Elastic: config.ElasticConfig{
			ProviderConfig: config.ProviderConfig{
				BaseURL: srv.URL,
				Enabled: true,
			},
			IndexPattern: "my-app-*",
		},
	}
	report := diagnostics.RunStartupChecks(context.Background(), cfg, nopLogger())

	es := findProvider(report, "elasticsearch")
	if es == nil {
		t.Fatal("elasticsearch provider missing from report")
	}
	if es.OK {
		t.Error("expected elasticsearch to fail when index pattern not found")
	}
}

func findProvider(r diagnostics.StartupReport, name string) *diagnostics.ProviderStatus {
	for i := range r.Providers {
		if r.Providers[i].Name == name {
			return &r.Providers[i]
		}
	}
	return nil
}
