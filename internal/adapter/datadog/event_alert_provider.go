package datadog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
)

const eventProviderName = "datadog-events"

type EventAlertProvider struct {
	baseURLs    []string
	apiKey      string
	appKey      string
	monitorTags []string
	httpClient  *http.Client
}

type searchEventsRequest struct {
	Filter searchEventsFilter `json:"filter"`
	Page   searchEventsPage   `json:"page"`
	Sort   string             `json:"sort"`
}

type searchEventsFilter struct {
	Query string `json:"query,omitempty"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type searchEventsPage struct {
	Limit int `json:"limit"`
}

type searchEventsResponse struct {
	Data []eventItem `json:"data"`
}

type eventItem struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Attributes eventItemAttributes `json:"attributes"`
}

type eventItemAttributes struct {
	Message    string            `json:"message"`
	Tags       []string          `json:"tags"`
	Timestamp  string            `json:"timestamp"`
	Attributes eventInnerContent `json:"attributes"`
}

type eventInnerContent struct {
	MonitorID     int64        `json:"monitor_id"`
	Service       string       `json:"service"`
	Status        string       `json:"status"`
	Title         string       `json:"title"`
	Tags          []string     `json:"tags"`
	Timestamp     int64        `json:"timestamp"`
	Monitor       eventMonitor `json:"monitor"`
	MonitorGroups []string     `json:"monitor_groups"`
}

type eventMonitor struct {
	ID    int64    `json:"id"`
	Name  string   `json:"name"`
	Query string   `json:"query"`
	Tags  []string `json:"tags"`
	Type  string   `json:"type"`
}

func NewEventAlertProvider(cfg config.DatadogConfig) EventAlertProvider {
	return EventAlertProvider{
		baseURLs:    buildBaseURLs(cfg.BaseURL),
		apiKey:      cfg.APIKey,
		appKey:      cfg.AppKey,
		monitorTags: append([]string(nil), cfg.MonitorTags...),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (p EventAlertProvider) Name() string {
	return eventProviderName
}

func (p EventAlertProvider) FetchAlerts(ctx context.Context, since time.Time) ([]domain.Event, error) {
	result, err := p.FetchAlertsDebug(ctx, since)
	if err != nil {
		return nil, err
	}

	return result.Events, nil
}

func (p EventAlertProvider) FetchAlertsDebug(ctx context.Context, since time.Time) (alertFetchResult, error) {
	body, err := json.Marshal(searchEventsRequest{
		Filter: searchEventsFilter{
			Query: "source:alert",
			From:  since.Format(time.RFC3339),
			To:    time.Now().UTC().Format(time.RFC3339),
		},
		Page: searchEventsPage{
			Limit: 100,
		},
		Sort: "timestamp",
	})
	if err != nil {
		return alertFetchResult{}, fmt.Errorf("marshal events request: %w", err)
	}

	var lastErr error

	for _, baseURL := range p.baseURLs {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			strings.TrimRight(baseURL, "/")+"/api/v2/events/search",
			bytes.NewReader(body),
		)
		if err != nil {
			return alertFetchResult{}, fmt.Errorf("create request: %w", err)
		}

		request.Header.Set("Accept", "application/json")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("DD-API-KEY", p.apiKey)
		request.Header.Set("DD-APPLICATION-KEY", p.appKey)

		response, err := p.httpClient.Do(request)
		if err != nil {
			return alertFetchResult{}, fmt.Errorf("perform request: %w", err)
		}

		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			lastErr = fmt.Errorf("unexpected status code %d from %s", response.StatusCode, baseURL)
			if shouldRetryWithFallback(response.StatusCode) {
				continue
			}

			return alertFetchResult{}, lastErr
		}

		var payload searchEventsResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			return alertFetchResult{}, fmt.Errorf("decode events response: %w", err)
		}
		response.Body.Close()

		events, debug := p.mapEventItems(payload.Data, since)

		return alertFetchResult{
			Events: events,
			Debug:  debug,
		}, nil
	}

	if lastErr != nil {
		return alertFetchResult{}, lastErr
	}

	return alertFetchResult{}, fmt.Errorf("no datadog base url configured")
}

func (p EventAlertProvider) mapEventItems(items []eventItem, since time.Time) ([]domain.Event, alertProviderDebug) {
	events := make([]domain.Event, 0, len(items))
	debug := alertProviderDebug{
		MonitorsFetched: len(items),
	}

	for _, item := range items {
		appendEventSamples(&debug, item)

		event, eventDebug, ok := p.mapEventItem(item, since)
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

func (p EventAlertProvider) mapEventItem(item eventItem, since time.Time) (domain.Event, monitorDebug, bool) {
	if item.Type != "event" {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}

	content := item.Attributes.Attributes
	if content.MonitorID == 0 && content.Monitor.ID == 0 {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}

	occurredAt, ok := parseEventTimestamp(item.Attributes, content)
	if !ok {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}
	if occurredAt.Before(since) {
		return domain.Event{}, monitorDebug{CandidateEvents: 1, IgnoredBeforeSince: 1}, false
	}

	severity, ok := mapEventStatusSeverity(content.Status)
	if !ok {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}

	allTags := mergeTags(item.Attributes.Tags, content.Tags, content.Monitor.Tags)
	if !matchesAllTags(allTags, p.monitorTags) {
		return domain.Event{}, monitorDebug{CandidateEvents: 1, IgnoredByStatus: 1}, false
	}

	service, environment, ownerTeam := extractMetadataFromTags(allTags)
	if service == "" {
		service = content.Service
	}
	queryMetadata := extractMetadataFromQuery(content.Monitor.Query)
	if service == "" {
		service = queryMetadata.Service
	}
	if environment == "" {
		environment = queryMetadata.Environment
	}

	monitorID := content.MonitorID
	if monitorID == 0 {
		monitorID = content.Monitor.ID
	}

	title := content.Title
	if title == "" {
		title = content.Monitor.Name
	}

	return domain.Event{
		ID:          item.ID,
		Source:      domain.SourceDatadog,
		Service:     service,
		Environment: environment,
		Severity:    severity,
		StatusCode:  queryMetadata.StatusCode,
		StatusClass: queryMetadata.StatusClass,
		Route:       queryMetadata.Route,
		Message:     title,
		OccurredAt:  occurredAt,
		Fingerprint: fmt.Sprintf("datadog-event:%d:%s", monitorID, item.ID),
		Summary:     title,
		OwnerTeam:   ownerTeam,
		Attributes: map[string]string{
			"event_id":   item.ID,
			"monitor_id": fmt.Sprintf("%d", monitorID),
			"status":     content.Status,
			"title":      title,
			"query":      content.Monitor.Query,
		},
	}, monitorDebug{CandidateEvents: 1}, true
}

func parseEventTimestamp(attributes eventItemAttributes, content eventInnerContent) (time.Time, bool) {
	if content.Timestamp > 0 {
		return time.UnixMilli(content.Timestamp).UTC(), true
	}
	if attributes.Timestamp == "" {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.RFC3339, attributes.Timestamp)
	if err != nil {
		return time.Time{}, false
	}

	return parsed.UTC(), true
}

func mapEventStatusSeverity(status string) (domain.Severity, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "alert":
		return domain.SeverityAlert, true
	case "warn", "warning", "no data":
		return domain.SeverityWarning, true
	case "info", "success":
		return domain.SeverityInfo, true
	default:
		return "", false
	}
}

func mergeTags(tagSets ...[]string) []string {
	tags := make([]string, 0)

	for _, tagSet := range tagSets {
		for _, tag := range tagSet {
			if !slices.Contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}

	return tags
}

func matchesAllTags(tags []string, requiredTags []string) bool {
	if len(requiredTags) == 0 {
		return true
	}

	for _, requiredTag := range requiredTags {
		if !slices.Contains(tags, requiredTag) {
			return false
		}
	}

	return true
}

func appendEventSamples(debug *alertProviderDebug, item eventItem) {
	title := item.Attributes.Attributes.Title
	if title == "" {
		title = item.Attributes.Message
	}
	if title != "" && len(debug.SampleMonitorNames) < 5 {
		debug.SampleMonitorNames = append(debug.SampleMonitorNames, title)
	}

	for _, tag := range mergeTags(item.Attributes.Tags, item.Attributes.Attributes.Tags, item.Attributes.Attributes.Monitor.Tags) {
		if len(debug.SampleTags) >= 8 {
			return
		}
		if !slices.Contains(debug.SampleTags, tag) {
			debug.SampleTags = append(debug.SampleTags, tag)
		}
	}
}
