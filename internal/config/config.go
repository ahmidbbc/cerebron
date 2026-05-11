package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTP        HTTPConfig
	Environment EnvironmentConfig
	Datadog     DatadogConfig
	Elastic     ElasticConfig
	Gerrit      ProviderConfig
	GitHub      ProviderConfig
	Slack       SlackConfig
	Storage     StorageConfig
}

type HTTPConfig struct {
	Address         string
	ShutdownTimeout time.Duration
}

type EnvironmentConfig struct {
	Name                string
	Mode                string
	ObservedEnvs        []string
	DefaultPollInterval time.Duration
}

type ProviderConfig struct {
	BaseURL string
	Token   string
	Enabled bool
}

type DatadogConfig struct {
	BaseURL       string
	APIKey        string
	AppKey        string
	Enabled       bool
	MonitorTags   []string
	ErrorTracking DatadogErrorTrackingConfig
}

type DatadogErrorTrackingConfig struct {
	BaseURL string
	Query   string
	Track   string
	Enabled bool
}

type ElasticConfig struct {
	ProviderConfig
	IndexPattern      string
	EnvironmentFields []string
}

type SlackConfig struct {
	ProviderConfig
	ChannelID string
}

type StorageConfig struct {
	PostgresURL string
	RedisAddr   string
}

func LoadFromEnv() (Config, error) {
	datadogBaseURL := getEnv("DATADOG_BASE_URL", "https://api.datadoghq.eu")
	cfg := Config{
		HTTP: HTTPConfig{
			Address:         getEnv("HTTP_ADDRESS", ":8080"),
			ShutdownTimeout: getDurationEnv("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Environment: EnvironmentConfig{
			Name:                getEnv("APP_ENV", "local"),
			Mode:                getEnv("APP_MODE", "test"),
			ObservedEnvs:        getListEnv("OBSERVED_ENVS", nil),
			DefaultPollInterval: getDurationEnv("DEFAULT_POLL_INTERVAL", time.Minute),
		},
		Datadog: DatadogConfig{
			BaseURL:     datadogBaseURL,
			APIKey:      getEnv("DATADOG_API_KEY", ""),
			AppKey:      getEnv("DATADOG_APP_KEY", ""),
			Enabled:     getBoolEnv("DATADOG_ENABLED", false),
			MonitorTags: getListEnv("DATADOG_MONITOR_TAGS", nil),
			ErrorTracking: DatadogErrorTrackingConfig{
				BaseURL: getEnv("DATADOG_ERROR_TRACKING_BASE_URL", datadogBaseURL),
				Query:   getEnv("DATADOG_ERROR_TRACKING_QUERY", ""),
				Track:   getEnv("DATADOG_ERROR_TRACKING_TRACK", "trace"),
				Enabled: getBoolEnv("DATADOG_ERROR_TRACKING_ENABLED", false),
			},
		},
		Elastic: ElasticConfig{
			ProviderConfig: ProviderConfig{
				BaseURL: getEnv("ELASTIC_BASE_URL", ""),
				Token:   getEnv("ELASTIC_TOKEN", ""),
				Enabled: getBoolEnv("ELASTIC_ENABLED", false),
			},
			IndexPattern:      getEnv("ELASTIC_INDEX_PATTERN", "*"),
			EnvironmentFields: getListEnv("ELASTIC_ENVIRONMENT_FIELDS", []string{"k8s-namespace", "service.environment", "environment", "env", "labels.env"}),
		},
		Gerrit: ProviderConfig{
			BaseURL: getEnv("GERRIT_BASE_URL", ""),
			Token:   getEnv("GERRIT_TOKEN", ""),
			Enabled: getBoolEnv("GERRIT_ENABLED", false),
		},
		GitHub: ProviderConfig{
			BaseURL: getEnv("GITHUB_BASE_URL", "https://api.github.com"),
			Token:   getEnv("GITHUB_TOKEN", ""),
			Enabled: getBoolEnv("GITHUB_ENABLED", false),
		},
		Slack: SlackConfig{
			ProviderConfig: ProviderConfig{
				BaseURL: getEnv("SLACK_BASE_URL", "https://slack.com/api"),
				Token:   getEnv("SLACK_BOT_TOKEN", ""),
				Enabled: getBoolEnv("SLACK_ENABLED", false),
			},
			ChannelID: getEnv("SLACK_CHANNEL_ID", ""),
		},
		Storage: StorageConfig{
			PostgresURL: getEnv("POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/cerebron?sslmode=disable"),
			RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		},
	}

	if cfg.Datadog.ErrorTracking.Query == "" {
		cfg.Datadog.ErrorTracking.Query = deriveDatadogErrorTrackingQuery(cfg.Datadog.MonitorTags)
	}

	if cfg.Slack.Enabled && cfg.Slack.ChannelID == "" {
		return Config{}, fmt.Errorf("SLACK_CHANNEL_ID is required when Slack is enabled")
	}
	if (cfg.Datadog.Enabled || cfg.Datadog.ErrorTracking.Enabled) && (cfg.Datadog.APIKey == "" || cfg.Datadog.AppKey == "") {
		return Config{}, fmt.Errorf("DATADOG_API_KEY and DATADOG_APP_KEY are required when Datadog is enabled")
	}
	if cfg.Datadog.ErrorTracking.Enabled && cfg.Datadog.ErrorTracking.Query == "" {
		return Config{}, fmt.Errorf("DATADOG_ERROR_TRACKING_QUERY or compatible DATADOG_MONITOR_TAGS are required when Datadog error tracking is enabled")
	}
	if cfg.Datadog.ErrorTracking.Enabled && !isValidDatadogErrorTrackingTrack(cfg.Datadog.ErrorTracking.Track) {
		return Config{}, fmt.Errorf("DATADOG_ERROR_TRACKING_TRACK must be one of trace, logs, rum")
	}
	if cfg.Elastic.Enabled && cfg.Elastic.BaseURL == "" {
		return Config{}, fmt.Errorf("ELASTIC_BASE_URL is required when Elasticsearch is enabled")
	}
	if cfg.Environment.Mode != "test" && cfg.Environment.Mode != "prod" {
		return Config{}, fmt.Errorf("APP_MODE must be test or prod")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func getListEnv(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	rawItems := strings.Split(value, ",")
	items := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}

	if len(items) == 0 {
		return fallback
	}

	return items
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return duration
}

func getBoolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func isValidDatadogErrorTrackingTrack(track string) bool {
	switch strings.ToLower(strings.TrimSpace(track)) {
	case "trace", "logs", "rum":
		return true
	default:
		return false
	}
}

func deriveDatadogErrorTrackingQuery(tags []string) string {
	filters := make([]string, 0, len(tags))

	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(key)) {
		case "env", "service":
			filters = append(filters, trimmed)
		}
	}

	return strings.Join(filters, " ")
}
