package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const providerName = "datadog"

var defaultBaseURLs = []string{
	"https://api.datadoghq.eu",
	"https://api.datadoghq.com",
}

var datadogQueryFieldPatterns = map[string]*regexp.Regexp{
	"service":      regexp.MustCompile(`(?:^|[\s,{])service:([^,\s}]+)`),
	"environment":  regexp.MustCompile(`(?:^|[\s,{])env:([^,\s}]+)`),
	"route":        regexp.MustCompile(`(?:^|[\s,{])http\.route:([^,\s}]+)`),
	"status_code":  regexp.MustCompile(`(?:^|[\s,{])http\.response\.status_code:(\d{3})`),
	"status_class": regexp.MustCompile(`(?:^|[\s,{])http\.response\.status_code:([1-5])\*`),
}

type AlertProvider struct {
	baseURLs    []string
	apiKey      string
	appKey      string
	monitorTags []string
	httpClient  *http.Client
}

type listMonitorsResponse []monitor

type monitor struct {
	ID                   int64        `json:"id"`
	Name                 string       `json:"name"`
	Type                 string       `json:"type"`
	Query                string       `json:"query"`
	Tags                 []string     `json:"tags"`
	OverallState         string       `json:"overall_state"`
	OverallStateModified string       `json:"overall_state_modified"`
	State                monitorState `json:"state"`
}

type monitorState struct {
	Groups map[string]monitorGroupState `json:"groups"`
}

type monitorGroupState struct {
	Status          string `json:"status"`
	LastNodataTS    int64  `json:"last_nodata_ts"`
	LastResolvedTS  int64  `json:"last_resolved_ts"`
	LastTriggeredTS int64  `json:"last_triggered_ts"`
}

func NewAlertProvider(cfg config.DatadogConfig) AlertProvider {
	return AlertProvider{
		baseURLs:    buildBaseURLs(cfg.BaseURL),
		apiKey:      cfg.APIKey,
		appKey:      cfg.AppKey,
		monitorTags: append([]string(nil), cfg.MonitorTags...),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (p AlertProvider) Name() string {
	return providerName
}

func (p AlertProvider) FetchAlerts(ctx context.Context, since time.Time) ([]domain.Event, error) {
	result, err := p.FetchAlertsDebug(ctx, since)
	if err != nil {
		return nil, err
	}

	return result.Events, nil
}

func (p AlertProvider) FetchAlertsDebug(ctx context.Context, since time.Time) (outbound.AlertFetchResult, error) {
	var lastErr error

	for _, baseURL := range p.baseURLs {
		endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/v1/monitor")
		if err != nil {
			return outbound.AlertFetchResult{}, fmt.Errorf("build monitors endpoint: %w", err)
		}

		queryValues := endpoint.Query()
		queryValues.Set("group_states", "alert,warn,no data")
		if len(p.monitorTags) > 0 {
			queryValues.Set("monitor_tags", strings.Join(p.monitorTags, ","))
		}
		endpoint.RawQuery = queryValues.Encode()

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return outbound.AlertFetchResult{}, fmt.Errorf("create request: %w", err)
		}

		request.Header.Set("Accept", "application/json")
		request.Header.Set("DD-API-KEY", p.apiKey)
		request.Header.Set("DD-APPLICATION-KEY", p.appKey)

		response, err := p.httpClient.Do(request)
		if err != nil {
			return outbound.AlertFetchResult{}, fmt.Errorf("perform request: %w", err)
		}

		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			lastErr = fmt.Errorf("unexpected status code %d from %s", response.StatusCode, baseURL)
			if shouldRetryWithFallback(response.StatusCode) {
				continue
			}
			return outbound.AlertFetchResult{}, lastErr
		}

		var payload listMonitorsResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			return outbound.AlertFetchResult{}, fmt.Errorf("decode monitors response: %w", err)
		}
		response.Body.Close()

		events := make([]domain.Event, 0)
		debug := outbound.AlertProviderDebug{
			MonitorsFetched: len(payload),
		}
		for _, monitor := range payload {
			appendSamples(&debug, monitor)
			monitorEvents, monitorDebug := mapMonitorEvents(monitor, since)
			events = append(events, monitorEvents...)
			debug.CandidateEvents += monitorDebug.CandidateEvents
			debug.IgnoredByStatus += monitorDebug.IgnoredByStatus
			debug.IgnoredBeforeSince += monitorDebug.IgnoredBeforeSince
		}

		return outbound.AlertFetchResult{
			Events: events,
			Debug:  debug,
		}, nil
	}

	if lastErr != nil {
		return outbound.AlertFetchResult{}, lastErr
	}

	return outbound.AlertFetchResult{}, fmt.Errorf("no datadog base url configured")
}

type monitorDebug struct {
	CandidateEvents    int
	IgnoredByStatus    int
	IgnoredBeforeSince int
}

func mapMonitorEvents(monitor monitor, since time.Time) ([]domain.Event, monitorDebug) {
	events := make([]domain.Event, 0)
	debug := monitorDebug{}

	if len(monitor.State.Groups) == 0 {
		event, eventDebug, ok := mapFallbackMonitorEvent(monitor, since)
		debug.CandidateEvents += eventDebug.CandidateEvents
		debug.IgnoredByStatus += eventDebug.IgnoredByStatus
		debug.IgnoredBeforeSince += eventDebug.IgnoredBeforeSince
		if ok {
			events = append(events, event)
		}
		return events, debug
	}

	for groupName, groupState := range monitor.State.Groups {
		event, eventDebug, ok := mapGroupMonitorEvent(monitor, groupName, groupState, since)
		debug.CandidateEvents += eventDebug.CandidateEvents
		debug.IgnoredByStatus += eventDebug.IgnoredByStatus
		debug.IgnoredBeforeSince += eventDebug.IgnoredBeforeSince
		if !ok {
			continue
		}

		events = append(events, event)
	}

	return events, debug
}

func mapGroupMonitorEvent(monitor monitor, groupName string, groupState monitorGroupState, since time.Time) (domain.Event, monitorDebug, bool) {
	occurredAt, severity, ok := mapMonitorState(groupState.Status, groupState.LastTriggeredTS, groupState.LastNodataTS)
	if !ok {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}
	if occurredAt.Before(since) {
		return domain.Event{}, monitorDebug{CandidateEvents: 1, IgnoredBeforeSince: 1}, false
	}

	service, environment, ownerTeam := extractMetadataFromTags(monitor.Tags)
	queryMetadata := extractMetadataFromQuery(monitor.Query)
	if service == "" {
		service = queryMetadata.Service
	}
	if environment == "" {
		environment = queryMetadata.Environment
	}
	fingerprint := fmt.Sprintf("datadog:%d:%s:%s", monitor.ID, sanitizeGroupName(groupName), strings.ToLower(string(severity)))

	return domain.Event{
		ID:          fingerprint,
		Source:      domain.SourceDatadog,
		Service:     service,
		Environment: environment,
		Severity:    severity,
		StatusCode:  queryMetadata.StatusCode,
		StatusClass: queryMetadata.StatusClass,
		Route:       queryMetadata.Route,
		Message:     monitor.Name,
		OccurredAt:  occurredAt,
		Fingerprint: fingerprint,
		Summary:     monitor.Name,
		OwnerTeam:   ownerTeam,
		Attributes: map[string]string{
			"group":   groupName,
			"status":  groupState.Status,
			"type":    monitor.Type,
			"query":   monitor.Query,
			"monitor": strconv.FormatInt(monitor.ID, 10),
		},
	}, monitorDebug{CandidateEvents: 1}, true
}

func mapFallbackMonitorEvent(monitor monitor, since time.Time) (domain.Event, monitorDebug, bool) {
	occurredAt, err := time.Parse(time.RFC3339, monitor.OverallStateModified)
	if err != nil {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}

	severity, ok := mapOverallStateSeverity(monitor.OverallState)
	if !ok {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}
	if occurredAt.Before(since) {
		return domain.Event{}, monitorDebug{CandidateEvents: 1, IgnoredBeforeSince: 1}, false
	}

	service, environment, ownerTeam := extractMetadataFromTags(monitor.Tags)
	queryMetadata := extractMetadataFromQuery(monitor.Query)
	if service == "" {
		service = queryMetadata.Service
	}
	if environment == "" {
		environment = queryMetadata.Environment
	}
	fingerprint := fmt.Sprintf("datadog:%d:%s", monitor.ID, strings.ToLower(string(severity)))

	return domain.Event{
		ID:          fingerprint,
		Source:      domain.SourceDatadog,
		Service:     service,
		Environment: environment,
		Severity:    severity,
		StatusCode:  queryMetadata.StatusCode,
		StatusClass: queryMetadata.StatusClass,
		Route:       queryMetadata.Route,
		Message:     monitor.Name,
		OccurredAt:  occurredAt,
		Fingerprint: fingerprint,
		Summary:     monitor.Name,
		OwnerTeam:   ownerTeam,
		Attributes: map[string]string{
			"status":  monitor.OverallState,
			"type":    monitor.Type,
			"query":   monitor.Query,
			"monitor": strconv.FormatInt(monitor.ID, 10),
		},
	}, monitorDebug{CandidateEvents: 1}, true
}

func mapMonitorState(status string, lastTriggeredTS int64, lastNodataTS int64) (time.Time, domain.Severity, bool) {
	switch status {
	case "Alert":
		if lastTriggeredTS == 0 {
			return time.Time{}, "", false
		}
		return time.Unix(lastTriggeredTS, 0).UTC(), domain.SeverityAlert, true
	case "Warn":
		if lastTriggeredTS == 0 {
			return time.Time{}, "", false
		}
		return time.Unix(lastTriggeredTS, 0).UTC(), domain.SeverityWarning, true
	case "No Data":
		if lastNodataTS == 0 {
			return time.Time{}, "", false
		}
		return time.Unix(lastNodataTS, 0).UTC(), domain.SeverityWarning, true
	default:
		return time.Time{}, "", false
	}
}

func mapOverallStateSeverity(state string) (domain.Severity, bool) {
	switch state {
	case "Alert":
		return domain.SeverityAlert, true
	case "Warn":
		return domain.SeverityWarning, true
	case "No Data":
		return domain.SeverityWarning, true
	default:
		return "", false
	}
}

func extractMetadataFromTags(tags []string) (string, string, string) {
	var service string
	var environment string
	var ownerTeam string

	for _, tag := range tags {
		key, value, ok := strings.Cut(tag, ":")
		if !ok {
			continue
		}

		switch key {
		case "service":
			service = value
		case "env":
			environment = value
		case "team":
			ownerTeam = value
		}
	}

	return service, environment, ownerTeam
}

type queryMetadata struct {
	Service     string
	Environment string
	Route       string
	StatusCode  int
	StatusClass string
}

func extractMetadataFromQuery(query string) queryMetadata {
	return queryMetadata{
		Service:     extractQueryField(query, "service"),
		Environment: extractQueryField(query, "environment"),
		Route:       extractQueryField(query, "route"),
		StatusCode:  extractQueryInt(query, "status_code"),
		StatusClass: extractStatusClass(query),
	}
}

func extractQueryField(query, field string) string {
	pattern, ok := datadogQueryFieldPatterns[field]
	if !ok {
		return ""
	}

	matches := pattern.FindStringSubmatch(query)
	if len(matches) != 2 {
		return ""
	}

	return matches[1]
}

func extractQueryInt(query, field string) int {
	value := extractQueryField(query, field)
	if value == "" {
		return 0
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}

	return parsed
}

func extractStatusClass(query string) string {
	if statusCode := extractQueryInt(query, "status_code"); statusCode > 0 {
		return statusClassFromCode(statusCode)
	}

	value := extractQueryField(query, "status_class")
	if value == "" {
		return ""
	}

	return value + "xx"
}

func statusClassFromCode(statusCode int) string {
	if statusCode < 100 || statusCode > 599 {
		return ""
	}

	return fmt.Sprintf("%dxx", statusCode/100)
}

func sanitizeGroupName(group string) string {
	replacer := strings.NewReplacer(":", "-", ",", "-", " ", "-", "*", "all")
	return replacer.Replace(group)
}

func appendSamples(debug *outbound.AlertProviderDebug, monitor monitor) {
	if len(debug.SampleMonitorNames) < 5 {
		debug.SampleMonitorNames = append(debug.SampleMonitorNames, monitor.Name)
	}

	for _, tag := range monitor.Tags {
		if len(debug.SampleTags) >= 8 {
			return
		}
		if !slices.Contains(debug.SampleTags, tag) {
			debug.SampleTags = append(debug.SampleTags, tag)
		}
	}
}

func buildBaseURLs(configuredBaseURL string) []string {
	baseURLs := make([]string, 0, 1+len(defaultBaseURLs))

	if trimmed := strings.TrimRight(strings.TrimSpace(configuredBaseURL), "/"); trimmed != "" {
		baseURLs = append(baseURLs, trimmed)
	}

	for _, candidate := range defaultBaseURLs {
		if !slices.Contains(baseURLs, candidate) {
			baseURLs = append(baseURLs, candidate)
		}
	}

	return baseURLs
}

func shouldRetryWithFallback(statusCode int) bool {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	default:
		return false
	}
}
