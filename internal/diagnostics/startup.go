package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cerebron/internal/config"
)

// ProviderStatus holds the startup check result for a single provider.
type ProviderStatus struct {
	Name    string
	Enabled bool
	OK      bool
	Err     error
}

// StartupReport summarises all provider statuses after startup checks.
type StartupReport struct {
	Providers []ProviderStatus
}

// HasErrors reports whether any enabled provider failed its startup check.
func (r StartupReport) HasErrors() bool {
	for _, p := range r.Providers {
		if p.Enabled && p.Err != nil {
			return true
		}
	}
	return false
}

// RunStartupChecks validates provider connectivity and configuration, logs the
// results, and returns a report. Failures are logged as warnings — startup is
// never aborted here; callers decide how to act on errors.
func RunStartupChecks(ctx context.Context, cfg config.Config, log *slog.Logger) StartupReport {
	client := &http.Client{Timeout: 10 * time.Second}
	report := StartupReport{}

	ddStatus := checkDatadog(ctx, cfg.Datadog, client)
	report.Providers = append(report.Providers, ddStatus...)

	esStatus := checkElasticsearch(ctx, cfg.Elastic, client)
	report.Providers = append(report.Providers, esStatus)

	logReport(log, report)
	return report
}

func checkDatadog(ctx context.Context, cfg config.DatadogConfig, client *http.Client) []ProviderStatus {
	enabled := cfg.Enabled
	errTrackingEnabled := cfg.ErrorTracking.Enabled

	// Both providers share the same auth; one connectivity check covers both.
	if !enabled && !errTrackingEnabled {
		return []ProviderStatus{
			{Name: "datadog/monitors", Enabled: false},
			{Name: "datadog/error-tracking", Enabled: false},
		}
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.datadoghq.eu"
	}

	err := datadogValidate(ctx, baseURL, cfg.APIKey, cfg.AppKey, client)

	monitorsStatus := ProviderStatus{Name: "datadog/monitors", Enabled: enabled}
	if enabled {
		monitorsStatus.OK = err == nil
		monitorsStatus.Err = err
	}

	errTrackingStatus := ProviderStatus{Name: "datadog/error-tracking", Enabled: errTrackingEnabled}
	if errTrackingEnabled {
		errTrackingStatus.OK = err == nil
		errTrackingStatus.Err = err
	}

	return []ProviderStatus{monitorsStatus, errTrackingStatus}
}

func datadogValidate(ctx context.Context, baseURL, apiKey, appKey string, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/validate", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-APPLICATION-KEY", appKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to datadog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("datadog auth rejected (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("datadog validate returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode datadog validate response: %w", err)
	}
	if !payload.Valid {
		return fmt.Errorf("datadog credentials invalid")
	}

	return nil
}

func checkElasticsearch(ctx context.Context, cfg config.ElasticConfig, client *http.Client) ProviderStatus {
	status := ProviderStatus{Name: "elasticsearch", Enabled: cfg.Enabled}
	if !cfg.Enabled {
		return status
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")

	// First probe cluster health.
	if err := elasticsearchHealth(ctx, baseURL, cfg.Token, client); err != nil {
		status.Err = err
		return status
	}

	// Then verify the index pattern resolves to at least one index.
	if err := elasticsearchIndexExists(ctx, baseURL, cfg.IndexPattern, cfg.Token, client); err != nil {
		status.Err = err
		return status
	}

	status.OK = true
	return status
}

func elasticsearchHealth(ctx context.Context, baseURL, token string, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/_cluster/health", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if auth := normalizeAuth(token); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to elasticsearch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("elasticsearch auth rejected (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("elasticsearch cluster health returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode cluster health response: %w", err)
	}
	if payload.Status == "red" {
		return fmt.Errorf("elasticsearch cluster status is red")
	}

	return nil
}

func elasticsearchIndexExists(ctx context.Context, baseURL, indexPattern, token string, client *http.Client) error {
	pattern := strings.Trim(indexPattern, "/")
	if pattern == "" || pattern == "*" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL+"/"+pattern, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if auth := normalizeAuth(token); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("check index pattern: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("elasticsearch index pattern %q not found", indexPattern)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("elasticsearch index check returned HTTP %d", resp.StatusCode)
	}

	return nil
}

func normalizeAuth(token string) string {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "apikey ") || strings.HasPrefix(lower, "bearer ") || strings.HasPrefix(lower, "basic ") {
		return trimmed
	}
	return "ApiKey " + trimmed
}

func logReport(log *slog.Logger, report StartupReport) {
	for _, p := range report.Providers {
		attrs := []any{
			slog.String("provider", p.Name),
			slog.Bool("enabled", p.Enabled),
		}
		if !p.Enabled {
			log.Info("startup: provider disabled", attrs...)
			continue
		}
		if p.Err != nil {
			log.Warn("startup: provider check failed",
				append(attrs, slog.String("error", p.Err.Error()))...)
			continue
		}
		log.Info("startup: provider ok", attrs...)
	}
}
