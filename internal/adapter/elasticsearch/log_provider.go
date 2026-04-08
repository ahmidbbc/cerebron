package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
)

const providerName = "elasticsearch"

var defaultServiceFields = []string{
	"application",
	"service.name",
	"service",
	"k8s-container",
	"kubernetes.labels.app",
	"kubernetes.container.name",
}

var defaultLogTextFields = []string{
	"error",
	"error.message",
	"message",
	"log.original",
}

type LogProvider struct {
	baseURL           string
	token             string
	indexPattern      string
	environmentFields []string
	httpClient        *http.Client
}

type searchRequest struct {
	Size  int                    `json:"size"`
	Sort  []map[string]sortOrder `json:"sort"`
	Query searchQuery            `json:"query"`
}

type sortOrder struct {
	Order string `json:"order"`
}

type searchQuery struct {
	Bool boolQuery `json:"bool"`
}

type boolQuery struct {
	Filter []any `json:"filter"`
}

type rangeFilter struct {
	Range map[string]rangeBounds `json:"range"`
}

type rangeBounds struct {
	GTE    string `json:"gte"`
	LTE    string `json:"lte"`
	Format string `json:"format"`
}

type searchResponse struct {
	Hits searchHits `json:"hits"`
}

type searchHits struct {
	Hits []searchHit `json:"hits"`
}

type searchHit struct {
	ID     string         `json:"_id"`
	Index  string         `json:"_index"`
	Source map[string]any `json:"_source"`
}

func NewLogProvider(cfg config.ElasticConfig) LogProvider {
	return LogProvider{
		baseURL:           strings.TrimRight(cfg.BaseURL, "/"),
		token:             strings.TrimSpace(cfg.Token),
		indexPattern:      strings.TrimSpace(cfg.IndexPattern),
		environmentFields: append([]string(nil), cfg.EnvironmentFields...),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (p LogProvider) Name() string {
	return providerName
}

func (p LogProvider) SearchLogs(ctx context.Context, query domain.LogQuery) ([]domain.Event, error) {
	filters := []any{
		rangeFilter{
			Range: map[string]rangeBounds{
				"@timestamp": {
					GTE:    query.Since.UTC().Format(time.RFC3339),
					LTE:    query.Until.UTC().Format(time.RFC3339),
					Format: "strict_date_optional_time",
				},
			},
		},
	}
	if environmentFilter := buildEnvironmentFilter(query.Environment, p.environmentFields); environmentFilter != nil {
		filters = append(filters, environmentFilter)
	}
	if serviceFilter := buildServiceFilter(query.Service, defaultServiceFields); serviceFilter != nil {
		filters = append(filters, serviceFilter)
	}
	if termsFilter := buildTermsFilter(query.Terms, defaultLogTextFields); termsFilter != nil {
		filters = append(filters, termsFilter)
	}

	requestBody := searchRequest{
		Size: 100,
		Sort: []map[string]sortOrder{
			{"@timestamp": {Order: "desc"}},
		},
		Query: searchQuery{
			Bool: boolQuery{
				Filter: filters,
			},
		},
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal elasticsearch query: %w", err)
	}

	endpoint := p.baseURL + "/" + p.searchPath()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create elasticsearch request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if authorization := normalizeAuthorizationHeader(p.token); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("perform elasticsearch request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d from %s", response.StatusCode, p.baseURL)
	}

	var searchResponse searchResponse
	if err := json.NewDecoder(response.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("decode elasticsearch response: %w", err)
	}

	events := make([]domain.Event, 0, len(searchResponse.Hits.Hits))
	for _, hit := range searchResponse.Hits.Hits {
		event, ok := p.mapSearchHit(hit)
		if !ok {
			continue
		}

		events = append(events, event)
	}

	return events, nil
}

func (p LogProvider) searchPath() string {
	indexPattern := p.indexPattern
	if indexPattern == "" {
		indexPattern = "*"
	}

	return strings.Trim(indexPattern, "/") + "/_search"
}

func normalizeAuthorizationHeader(token string) string {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ""
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "apikey "), strings.HasPrefix(lower, "bearer "), strings.HasPrefix(lower, "basic "):
		return trimmed
	default:
		return "ApiKey " + trimmed
	}
}

func (p LogProvider) mapSearchHit(hit searchHit) (domain.Event, bool) {
	occurredAt, ok := extractTime(hit.Source, "@timestamp", "timestamp", "time")
	if !ok {
		return domain.Event{}, false
	}

	statusCode := extractStatusCode(hit.Source)
	statusClass := statusClassFromCode(statusCode)
	severity, ok := extractSeverity(hit.Source, statusCode)
	if !ok {
		return domain.Event{}, false
	}

	service := firstString(hit.Source,
		"application",
		"service.name",
		"service",
		"k8s-container",
		"kubernetes.labels.app",
		"kubernetes.container.name",
	)
	environment := firstString(hit.Source, p.environmentFields...)
	ownerTeam := firstString(hit.Source,
		"team",
		"labels.team",
		"service.team",
	)
	summary := firstString(hit.Source,
		"error",
		"message",
		"log.original",
		"error.message",
	)
	if summary == "" {
		summary = fmt.Sprintf("Elasticsearch log event from %s", hit.Index)
	}

	fingerprint := firstString(hit.Source, "error.type", "log.logger", "logger")
	if fingerprint == "" {
		fingerprint = summary
	}

	route := firstString(hit.Source,
		"request-url",
		"http.route",
		"url.path",
		"path",
	)
	errorMessage := firstString(hit.Source,
		"error",
		"error.message",
	)

	return domain.Event{
		ID:          hit.ID,
		Source:      domain.SourceElasticsearch,
		Service:     service,
		Environment: environment,
		Severity:    severity,
		StatusCode:  statusCode,
		StatusClass: statusClass,
		Route:       route,
		Message:     firstString(hit.Source, "message", "log.original"),
		Error:       errorMessage,
		OccurredAt:  occurredAt,
		Fingerprint: fmt.Sprintf("elasticsearch:%s:%s", hit.Index, fingerprint),
		Summary:     summary,
		OwnerTeam:   ownerTeam,
		Attributes: map[string]string{
			"index":        hit.Index,
			"level":        firstString(hit.Source, "log.level", "level", "status"),
			"logger":       firstString(hit.Source, "log.logger", "logger"),
			"trace":        firstString(hit.Source, "trace.id"),
			"service":      service,
			"status_code":  strconv.Itoa(statusCode),
			"status_class": statusClass,
			"route":        route,
		},
	}, true
}

func buildEnvironmentFilter(environment string, fields []string) any {
	trimmedEnvironment := strings.TrimSpace(strings.ToLower(environment))
	if trimmedEnvironment == "" || len(fields) == 0 {
		return nil
	}

	clauses := make([]any, 0, len(fields)*2)
	for _, field := range fields {
		trimmedField := strings.TrimSpace(field)
		if trimmedField == "" {
			continue
		}

		for _, prefix := range environmentPrefixes(trimmedEnvironment) {
			clauses = append(clauses,
				map[string]any{
					"prefix": map[string]any{
						trimmedField: prefix,
					},
				},
				map[string]any{
					"prefix": map[string]any{
						trimmedField + ".keyword": prefix,
					},
				},
			)
		}
	}
	if len(clauses) == 0 {
		return nil
	}

	return map[string]any{
		"bool": map[string]any{
			"should":               clauses,
			"minimum_should_match": 1,
		},
	}
}

func buildServiceFilter(service string, fields []string) any {
	trimmedService := strings.TrimSpace(service)
	if trimmedService == "" || len(fields) == 0 {
		return nil
	}

	clauses := make([]any, 0, len(fields)*2)
	for _, field := range fields {
		trimmedField := strings.TrimSpace(field)
		if trimmedField == "" {
			continue
		}
		clauses = append(clauses,
			map[string]any{
				"term": map[string]any{
					trimmedField: trimmedService,
				},
			},
			map[string]any{
				"term": map[string]any{
					trimmedField + ".keyword": trimmedService,
				},
			},
		)
	}
	if len(clauses) == 0 {
		return nil
	}

	return map[string]any{
		"bool": map[string]any{
			"should":               clauses,
			"minimum_should_match": 1,
		},
	}
}

func buildTermsFilter(terms []string, fields []string) any {
	normalizedTerms := normalizeSearchTerms(terms)
	if len(normalizedTerms) == 0 || len(fields) == 0 {
		return nil
	}

	clauses := make([]any, 0, len(normalizedTerms)*len(fields))
	for _, term := range normalizedTerms {
		for _, field := range fields {
			trimmedField := strings.TrimSpace(field)
			if trimmedField == "" {
				continue
			}
			clauses = append(clauses, map[string]any{
				"match_phrase": map[string]any{
					trimmedField: term,
				},
			})
		}
	}
	if len(clauses) == 0 {
		return nil
	}

	return map[string]any{
		"bool": map[string]any{
			"should":               clauses,
			"minimum_should_match": 1,
		},
	}
}

func normalizeSearchTerms(terms []string) []string {
	normalized := make([]string, 0, len(terms))
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if len(trimmed) < 8 {
			continue
		}
		if slices.Contains(normalized, trimmed) {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	return normalized
}

func environmentPrefixes(environment string) []string {
	switch environmentFamily(environment) {
	case "prod":
		return []string{"prod", "production"}
	case "preprod":
		return []string{"preprod"}
	case "qa":
		return []string{"qa"}
	default:
		return []string{environment}
	}
}

func environmentFamily(environment string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(environment)); {
	case normalized == "":
		return ""
	case strings.HasPrefix(normalized, "preprod"):
		return "preprod"
	case normalized == "prod" || normalized == "production" || strings.HasPrefix(normalized, "prod-") || strings.HasPrefix(normalized, "prod_") || strings.HasPrefix(normalized, "production-") || strings.HasPrefix(normalized, "production_"):
		return "prod"
	case normalized == "qa" || strings.HasPrefix(normalized, "qa"):
		return "qa"
	default:
		return normalized
	}
}

func extractSeverity(source map[string]any, statusCode int) (domain.Severity, bool) {
	switch {
	case statusCode >= 500:
		return domain.SeverityAlert, true
	case statusCode >= 400:
		return domain.SeverityWarning, true
	}

	value := strings.ToLower(strings.TrimSpace(firstString(source, "log.level", "level", "status")))

	switch value {
	case "fatal", "panic", "critical", "crit":
		return domain.SeverityCritical, true
	case "error", "err":
		return domain.SeverityAlert, true
	case "warn", "warning":
		return domain.SeverityWarning, true
	default:
		return "", false
	}
}

func extractStatusCode(source map[string]any) int {
	for _, key := range []string{"http.response.status_code", "status", "status_code"} {
		value, ok := nestedValue(source, key)
		if !ok {
			continue
		}

		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case int64:
			return int(typed)
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return parsed
			}
		}
	}

	return 0
}

func statusClassFromCode(statusCode int) string {
	if statusCode < 100 || statusCode > 599 {
		return ""
	}

	return fmt.Sprintf("%dxx", statusCode/100)
}

func extractTime(source map[string]any, keys ...string) (time.Time, bool) {
	for _, key := range keys {
		value, ok := nestedValue(source, key)
		if !ok {
			continue
		}

		switch typed := value.(type) {
		case string:
			parsed, err := time.Parse(time.RFC3339, typed)
			if err == nil {
				return parsed.UTC(), true
			}
		}
	}

	return time.Time{}, false
}

func firstString(source map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := nestedValue(source, key)
		if !ok {
			continue
		}

		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return trimmed
			}
		case map[string]any:
			if nameValue, ok := typed["name"].(string); ok {
				trimmed := strings.TrimSpace(nameValue)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}

	return ""
}

func nestedValue(source map[string]any, key string) (any, bool) {
	current := any(source)

	for _, part := range strings.Split(key, ".") {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		next, ok := asMap[part]
		if !ok {
			return nil, false
		}

		current = next
	}

	return current, true
}
