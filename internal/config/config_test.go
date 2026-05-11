package config

import "testing"

func TestGetListEnvFallsBackWhenOnlySeparatorsAreProvided(t *testing.T) {
	t.Setenv("LIST_TEST_ENV", ", ,")

	values := getListEnv("LIST_TEST_ENV", []string{"qa", "preprod"})
	if len(values) != 2 {
		t.Fatalf("expected fallback values, got %v", values)
	}
}

func TestGetBoolEnvFallbackOnInvalidValue(t *testing.T) {
	t.Setenv("BOOL_TEST_ENV", "maybe")

	value := getBoolEnv("BOOL_TEST_ENV", true)
	if !value {
		t.Fatalf("expected fallback boolean value")
	}
}

func TestLoadFromEnvAllowsElasticWithoutToken(t *testing.T) {
	t.Setenv("APP_MODE", "test")
	t.Setenv("ELASTIC_ENABLED", "true")
	t.Setenv("ELASTIC_BASE_URL", "http://es-log-master.infra.stg.leboncoin.io:9200")
	t.Setenv("ELASTIC_TOKEN", "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected elastic config without token to load, got %v", err)
	}
	if cfg.Elastic.Token != "" {
		t.Fatalf("expected empty elastic token, got %q", cfg.Elastic.Token)
	}
}

func TestLoadFromEnvLoadsElasticEnvironmentFields(t *testing.T) {
	t.Setenv("APP_MODE", "test")
	t.Setenv("ELASTIC_ENVIRONMENT_FIELDS", "k8s-namespace,service.environment")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected elastic config to load, got %v", err)
	}
	if len(cfg.Elastic.EnvironmentFields) != 2 {
		t.Fatalf("expected 2 elastic environment fields, got %v", cfg.Elastic.EnvironmentFields)
	}
	if cfg.Elastic.EnvironmentFields[0] != "k8s-namespace" {
		t.Fatalf("expected first elastic environment field to be k8s-namespace, got %q", cfg.Elastic.EnvironmentFields[0])
	}
	if cfg.Elastic.EnvironmentFields[1] != "service.environment" {
		t.Fatalf("expected second elastic environment field to be service.environment, got %q", cfg.Elastic.EnvironmentFields[1])
	}
}

func TestLoadFromEnvLoadsDatadogErrorTrackingConfig(t *testing.T) {
	t.Setenv("APP_MODE", "test")
	t.Setenv("DATADOG_API_KEY", "api-key")
	t.Setenv("DATADOG_APP_KEY", "app-key")
	t.Setenv("DATADOG_ERROR_TRACKING_ENABLED", "true")
	t.Setenv("DATADOG_ERROR_TRACKING_QUERY", "service:presence-api env:preprod")
	t.Setenv("DATADOG_ERROR_TRACKING_TRACK", "trace")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected datadog error tracking config to load, got %v", err)
	}
	if !cfg.Datadog.ErrorTracking.Enabled {
		t.Fatalf("expected error tracking to be enabled")
	}
	if cfg.Datadog.ErrorTracking.Query != "service:presence-api env:preprod" {
		t.Fatalf("expected error tracking query to be loaded, got %q", cfg.Datadog.ErrorTracking.Query)
	}
	if cfg.Datadog.ErrorTracking.Track != "trace" {
		t.Fatalf("expected error tracking track trace, got %q", cfg.Datadog.ErrorTracking.Track)
	}
}

func TestLoadFromEnvDerivesDatadogErrorTrackingQueryFromMonitorTags(t *testing.T) {
	t.Setenv("APP_MODE", "test")
	t.Setenv("DATADOG_API_KEY", "api-key")
	t.Setenv("DATADOG_APP_KEY", "app-key")
	t.Setenv("DATADOG_ERROR_TRACKING_ENABLED", "true")
	t.Setenv("DATADOG_MONITOR_TAGS", "env:preprod,service:presence-api,team:identity")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected datadog error tracking config to load from monitor tags, got %v", err)
	}
	if cfg.Datadog.ErrorTracking.Query != "env:preprod service:presence-api" {
		t.Fatalf("expected derived error tracking query, got %q", cfg.Datadog.ErrorTracking.Query)
	}
}

func TestLoadFromEnvRequiresDatadogErrorTrackingQueryOrCompatibleMonitorTagsWhenEnabled(t *testing.T) {
	t.Setenv("APP_MODE", "test")
	t.Setenv("DATADOG_API_KEY", "api-key")
	t.Setenv("DATADOG_APP_KEY", "app-key")
	t.Setenv("DATADOG_ERROR_TRACKING_ENABLED", "true")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatalf("expected validation error when error tracking query and compatible monitor tags are missing")
	}
}
