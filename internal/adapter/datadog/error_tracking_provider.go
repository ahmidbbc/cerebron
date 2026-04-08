package datadog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const errorTrackingProviderName = "datadog-error-tracking"

type ErrorTrackingProvider struct {
	baseURLs   []string
	apiKey     string
	appKey     string
	query      string
	track      string
	httpClient *http.Client
}

type searchIssuesRequest struct {
	Data searchIssuesRequestData `json:"data"`
}

type spanSearchRequest struct {
	Data spanSearchRequestData `json:"data"`
}

type searchIssuesRequestData struct {
	Attributes searchIssuesRequestAttributes `json:"attributes"`
	Type       string                        `json:"type"`
}

type spanSearchRequestData struct {
	Attributes spanSearchRequestAttributes `json:"attributes"`
	Type       string                      `json:"type"`
}

type searchIssuesRequestAttributes struct {
	Query string `json:"query"`
	From  int64  `json:"from"`
	To    int64  `json:"to"`
	Track string `json:"track"`
}

type spanSearchRequestAttributes struct {
	Filter spanSearchFilter `json:"filter"`
	Page   spanSearchPage   `json:"page"`
	Sort   string           `json:"sort,omitempty"`
}

type spanSearchFilter struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Query string `json:"query"`
}

type spanSearchPage struct {
	Limit int `json:"limit"`
}

type searchIssuesResponse struct {
	Data     []searchIssueResult       `json:"data"`
	Included []searchIssueIncludedItem `json:"included"`
}

type spanSearchResponse struct {
	Data []spanSearchItem `json:"data"`
}

type getIssueResponse struct {
	Data     searchIssueIncludedItem   `json:"data"`
	Included []searchIssueIncludedItem `json:"included"`
}

type spanSearchItem struct {
	ID         string                 `json:"id"`
	Attributes map[string]any         `json:"attributes"`
	Extra      map[string]interface{} `json:"-"`
}

type searchIssueResult struct {
	ID            string                   `json:"id"`
	Type          string                   `json:"type"`
	Relationships searchIssueRelationships `json:"relationships"`
}

type searchIssueRelationships struct {
	Issue relatedResourceLink `json:"issue"`
}

type relatedResourceLink struct {
	Data relatedResource `json:"data"`
}

type searchIssueIncludedItem struct {
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Attributes    json.RawMessage            `json:"attributes"`
	Relationships issueIncludedRelationships `json:"relationships"`
}

type errorTrackingIssueAttributes struct {
	ErrorMessage string `json:"error_message"`
	ErrorType    string `json:"error_type"`
	FilePath     string `json:"file_path"`
	FunctionName string `json:"function_name"`
	FirstSeen    int64  `json:"first_seen"`
	LastSeen     int64  `json:"last_seen"`
	Platform     string `json:"platform"`
	Service      string `json:"service"`
	State        string `json:"state"`
	IsCrash      bool   `json:"is_crash"`
}

type issueIncludedRelationships struct {
	TeamOwners relatedResourceCollection `json:"team_owners"`
}

type relatedResourceCollection struct {
	Data []relatedResource `json:"data"`
}

type relatedResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type teamAttributes struct {
	Handle string `json:"handle"`
	Name   string `json:"name"`
}

type errorTrackingIssue struct {
	ID            string
	Attributes    errorTrackingIssueAttributes
	Relationships issueIncludedRelationships
}

type errorTrackingTeam struct {
	ID     string
	Handle string
	Name   string
}

type errorTrackingSpanContext struct {
	OccurredAt time.Time
	StatusCode int
	Route      string
	Message    string
	Error      string
}

func NewErrorTrackingProvider(cfg config.DatadogConfig) ErrorTrackingProvider {
	return ErrorTrackingProvider{
		baseURLs: buildBaseURLs(cfg.ErrorTracking.BaseURL),
		apiKey:   cfg.APIKey,
		appKey:   cfg.AppKey,
		query:    strings.TrimSpace(cfg.ErrorTracking.Query),
		track:    strings.TrimSpace(cfg.ErrorTracking.Track),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (p ErrorTrackingProvider) Name() string {
	return errorTrackingProviderName
}

func (p ErrorTrackingProvider) FetchAlerts(ctx context.Context, since time.Time) ([]domain.Event, error) {
	result, err := p.FetchAlertsDebug(ctx, since)
	if err != nil {
		return nil, err
	}

	return result.Events, nil
}

func (p ErrorTrackingProvider) FetchAlertsDebug(ctx context.Context, since time.Time) (outbound.AlertFetchResult, error) {
	body, err := json.Marshal(searchIssuesRequest{
		Data: searchIssuesRequestData{
			Attributes: searchIssuesRequestAttributes{
				Query: p.query,
				From:  since.UnixMilli(),
				To:    time.Now().UTC().UnixMilli(),
				Track: p.track,
			},
			Type: "search_request",
		},
	})
	if err != nil {
		return outbound.AlertFetchResult{}, fmt.Errorf("marshal error tracking search request: %w", err)
	}

	var lastErr error

	for _, baseURL := range p.baseURLs {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			strings.TrimRight(baseURL, "/")+"/api/v2/error-tracking/issues/search",
			bytes.NewReader(body),
		)
		if err != nil {
			return outbound.AlertFetchResult{}, fmt.Errorf("create request: %w", err)
		}

		request.Header.Set("Accept", "application/json")
		request.Header.Set("Content-Type", "application/json")
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

		var payload searchIssuesResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			return outbound.AlertFetchResult{}, fmt.Errorf("decode error tracking response: %w", err)
		}
		response.Body.Close()

		events, debug := p.mapIssues(ctx, payload, since)

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

func (p ErrorTrackingProvider) mapIssues(ctx context.Context, payload searchIssuesResponse, since time.Time) ([]domain.Event, outbound.AlertProviderDebug) {
	issues, teams := indexErrorTrackingIncludedItems(payload.Included)
	events := make([]domain.Event, 0, len(payload.Data))
	debug := outbound.AlertProviderDebug{
		MonitorsFetched: len(payload.Data),
	}
	queryMetadata := extractMetadataFromQuery(p.query)

	for _, item := range payload.Data {
		issueID := issueReferenceID(item)
		if !isErrorTrackingSearchResult(item.Type) {
			debug.IgnoredByStatus++
			continue
		}
		issue, ok := issues[issueID]
		if !ok {
			resolvedIssue, resolvedTeams, err := p.fetchIssueDetails(ctx, issueID)
			if err == nil {
				issue = resolvedIssue
				ok = true
				for teamID, team := range resolvedTeams {
					teams[teamID] = team
				}
			}
		}
		if !ok {
			debug.IgnoredByStatus++
			continue
		}

		appendErrorTrackingSamples(&debug, issue, queryMetadata, p.track)
		event, eventDebug, ok := p.mapIssue(ctx, item, issue, teams, queryMetadata, since)
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

func (p ErrorTrackingProvider) fetchIssueDetails(ctx context.Context, issueID string) (errorTrackingIssue, map[string]errorTrackingTeam, error) {
	if issueID == "" {
		return errorTrackingIssue{}, nil, fmt.Errorf("empty issue id")
	}

	var lastErr error

	for _, baseURL := range p.baseURLs {
		endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/v2/error-tracking/issues/" + issueID)
		if err != nil {
			return errorTrackingIssue{}, nil, fmt.Errorf("build issue details endpoint: %w", err)
		}
		queryValues := endpoint.Query()
		queryValues.Set("include", "team_owners")
		endpoint.RawQuery = queryValues.Encode()

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return errorTrackingIssue{}, nil, fmt.Errorf("create issue details request: %w", err)
		}

		request.Header.Set("Accept", "application/json")
		request.Header.Set("DD-API-KEY", p.apiKey)
		request.Header.Set("DD-APPLICATION-KEY", p.appKey)

		response, err := p.httpClient.Do(request)
		if err != nil {
			return errorTrackingIssue{}, nil, fmt.Errorf("perform issue details request: %w", err)
		}

		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			lastErr = fmt.Errorf("unexpected status code %d from %s", response.StatusCode, baseURL)
			if shouldRetryWithFallback(response.StatusCode) {
				continue
			}

			return errorTrackingIssue{}, nil, lastErr
		}

		var payload getIssueResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			return errorTrackingIssue{}, nil, fmt.Errorf("decode issue details response: %w", err)
		}
		response.Body.Close()

		includedItems := append([]searchIssueIncludedItem{payload.Data}, payload.Included...)
		issues, teams := indexErrorTrackingIncludedItems(includedItems)
		issue, ok := issues[issueID]
		if !ok {
			return errorTrackingIssue{}, nil, fmt.Errorf("issue %s missing from details response", issueID)
		}

		return issue, teams, nil
	}

	if lastErr != nil {
		return errorTrackingIssue{}, nil, lastErr
	}

	return errorTrackingIssue{}, nil, fmt.Errorf("no datadog base url configured")
}

func issueReferenceID(item searchIssueResult) string {
	if item.Relationships.Issue.Data.ID != "" {
		return item.Relationships.Issue.Data.ID
	}

	return item.ID
}

func isErrorTrackingSearchResult(itemType string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(itemType))

	return trimmed == "" || trimmed == "error_tracking_search_result"
}

func (p ErrorTrackingProvider) mapIssue(
	ctx context.Context,
	item searchIssueResult,
	issue errorTrackingIssue,
	teams map[string]errorTrackingTeam,
	queryMetadata queryMetadata,
	since time.Time,
) (domain.Event, monitorDebug, bool) {
	occurredAt, ok := errorTrackingTimestamp(issue.Attributes)
	if !ok {
		return domain.Event{}, monitorDebug{IgnoredByStatus: 1}, false
	}
	if occurredAt.Before(since) {
		return domain.Event{}, monitorDebug{CandidateEvents: 1, IgnoredBeforeSince: 1}, false
	}

	service := issue.Attributes.Service
	if service == "" {
		service = queryMetadata.Service
	}

	environment := queryMetadata.Environment
	summary := strings.TrimSpace(issue.Attributes.ErrorMessage)
	if summary == "" {
		summary = strings.TrimSpace(issue.Attributes.ErrorType)
	}
	if summary == "" {
		summary = service
	}

	fingerprint := fmt.Sprintf("datadog-error-tracking:%s", item.ID)
	spanContext, hasSpanContext := p.fetchSpanContext(ctx, issue, queryMetadata)
	if hasSpanContext {
		occurredAt = spanContext.OccurredAt
	}

	statusCode := 0
	route := ""
	if hasSpanContext {
		statusCode = spanContext.StatusCode
		route = spanContext.Route
	}

	message := strings.TrimSpace(issue.Attributes.ErrorMessage)
	if hasSpanContext && spanContext.Message != "" {
		message = spanContext.Message
	}

	errorValue := joinErrorTypeAndMessage(issue.Attributes.ErrorType, issue.Attributes.ErrorMessage)
	if hasSpanContext && spanContext.Error != "" {
		errorValue = spanContext.Error
	}

	attributes := map[string]string{
		"issue_id":   item.ID,
		"query":      p.query,
		"track":      p.track,
		"state":      issue.Attributes.State,
		"platform":   issue.Attributes.Platform,
		"error_type": issue.Attributes.ErrorType,
		"file_path":  issue.Attributes.FilePath,
		"function":   issue.Attributes.FunctionName,
		"first_seen": formatUnixMilli(issue.Attributes.FirstSeen),
		"last_seen":  formatUnixMilli(issue.Attributes.LastSeen),
	}
	if hasSpanContext && spanContext.Route != "" {
		attributes["span_route"] = spanContext.Route
	}
	if hasSpanContext && spanContext.StatusCode > 0 {
		attributes["span_status_code"] = fmt.Sprintf("%d", spanContext.StatusCode)
	}

	return domain.Event{
		ID:          fingerprint,
		Source:      domain.SourceDatadog,
		Service:     service,
		Environment: environment,
		Severity:    mapErrorTrackingSeverity(issue.Attributes.State, issue.Attributes.IsCrash),
		StatusCode:  statusCode,
		StatusClass: statusClassFromCode(statusCode),
		Route:       route,
		Message:     message,
		Error:       errorValue,
		OccurredAt:  occurredAt,
		Fingerprint: fingerprint,
		Summary:     summary,
		OwnerTeam:   resolveOwnerTeam(issue.Relationships, teams),
		Attributes:  attributes,
	}, monitorDebug{CandidateEvents: 1}, true
}

func (p ErrorTrackingProvider) fetchSpanContext(ctx context.Context, issue errorTrackingIssue, queryMetadata queryMetadata) (errorTrackingSpanContext, bool) {
	service := strings.TrimSpace(issue.Attributes.Service)
	if service == "" {
		service = strings.TrimSpace(queryMetadata.Service)
	}
	if service == "" {
		return errorTrackingSpanContext{}, false
	}

	from := time.UnixMilli(issue.Attributes.FirstSeen).UTC()
	if from.IsZero() {
		from = time.UnixMilli(issue.Attributes.LastSeen).UTC().Add(-5 * time.Minute)
	}
	to := time.UnixMilli(issue.Attributes.LastSeen).UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() || !from.Before(to) {
		from = to.Add(-15 * time.Minute)
	}
	to = to.Add(5 * time.Minute)

	body, err := json.Marshal(spanSearchRequest{
		Data: spanSearchRequestData{
			Attributes: spanSearchRequestAttributes{
				Filter: spanSearchFilter{
					From:  from.Format(time.RFC3339),
					To:    to.Format(time.RFC3339),
					Query: buildSpanSearchQuery(service, queryMetadata.Environment, issue.Attributes),
				},
				Page: spanSearchPage{
					Limit: 1,
				},
				Sort: "timestamp",
			},
			Type: "search_request",
		},
	})
	if err != nil {
		return errorTrackingSpanContext{}, false
	}

	for _, baseURL := range p.baseURLs {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			strings.TrimRight(baseURL, "/")+"/api/v2/spans/events/search",
			bytes.NewReader(body),
		)
		if err != nil {
			return errorTrackingSpanContext{}, false
		}

		request.Header.Set("Accept", "application/json")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("DD-API-KEY", p.apiKey)
		request.Header.Set("DD-APPLICATION-KEY", p.appKey)

		response, err := p.httpClient.Do(request)
		if err != nil {
			return errorTrackingSpanContext{}, false
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			if shouldRetryWithFallback(response.StatusCode) {
				continue
			}

			return errorTrackingSpanContext{}, false
		}

		var payload spanSearchResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			return errorTrackingSpanContext{}, false
		}
		response.Body.Close()

		for _, item := range payload.Data {
			context, ok := mapSpanSearchContext(item)
			if ok {
				return context, true
			}
		}

		return errorTrackingSpanContext{}, false
	}

	return errorTrackingSpanContext{}, false
}

func buildSpanSearchQuery(service string, environment string, attributes errorTrackingIssueAttributes) string {
	parts := []string{"service:" + service}
	if trimmedEnvironment := strings.TrimSpace(environment); trimmedEnvironment != "" {
		parts = append(parts, "env:"+trimmedEnvironment)
	}
	if trimmedErrorType := strings.TrimSpace(attributes.ErrorType); trimmedErrorType != "" {
		parts = append(parts, "@error.type:"+quoteDatadogQueryValue(trimmedErrorType))
	} else if trimmedMessage := strings.TrimSpace(attributes.ErrorMessage); trimmedMessage != "" {
		parts = append(parts, "@error.message:"+quoteDatadogQueryValue(trimmedMessage))
	}

	return strings.Join(parts, " ")
}

func quoteDatadogQueryValue(value string) string {
	return `"` + strings.ReplaceAll(strings.TrimSpace(value), `"`, `\"`) + `"`
}

func mapSpanSearchContext(item spanSearchItem) (errorTrackingSpanContext, bool) {
	attributes := item.Attributes
	if len(attributes) == 0 {
		return errorTrackingSpanContext{}, false
	}

	nestedAttributes := nestedAttributesMap(attributes)
	occurredAt, ok := extractSpanTimestamp(attributes, nestedAttributes)
	if !ok {
		return errorTrackingSpanContext{}, false
	}

	statusCode := extractSpanStatusCode(attributes, nestedAttributes)
	route := firstNonEmptyString(
		findString(nestedAttributes, "http.route"),
		normalizeSpanResourceRoute(findString(attributes, "resource_name")),
		findString(attributes, "resource_name"),
		normalizeSpanResourceRoute(findString(nestedAttributes, "resource_name")),
	)
	message := firstNonEmptyString(
		findString(nestedAttributes, "error.message"),
		findString(attributes, "error.message"),
	)
	errorValue := firstNonEmptyString(
		joinErrorTypeAndMessage(findString(nestedAttributes, "error.type"), message),
		joinErrorTypeAndMessage(findString(attributes, "error.type"), message),
	)

	return errorTrackingSpanContext{
		OccurredAt: occurredAt,
		StatusCode: statusCode,
		Route:      route,
		Message:    message,
		Error:      errorValue,
	}, true
}

func nestedAttributesMap(attributes map[string]any) map[string]any {
	rawAttributes, ok := attributes["attributes"]
	if !ok {
		return nil
	}

	nested, ok := rawAttributes.(map[string]any)
	if !ok {
		return nil
	}

	return nested
}

func extractSpanTimestamp(attributes map[string]any, nested map[string]any) (time.Time, bool) {
	for _, candidate := range []string{
		findString(attributes, "timestamp"),
		findString(attributes, "start_timestamp"),
		findString(nested, "timestamp"),
		findString(nested, "start_timestamp"),
	} {
		if candidate == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, candidate); err == nil {
			return parsed.UTC(), true
		}
	}

	for _, candidate := range []int64{
		findInt64(attributes, "timestamp"),
		findInt64(attributes, "start_timestamp"),
		findInt64(nested, "timestamp"),
		findInt64(nested, "start_timestamp"),
	} {
		if candidate <= 0 {
			continue
		}
		if candidate > 1_000_000_000_000 {
			return time.UnixMilli(candidate).UTC(), true
		}

		return time.Unix(candidate, 0).UTC(), true
	}

	return time.Time{}, false
}

func extractSpanStatusCode(attributes map[string]any, nested map[string]any) int {
	for _, source := range []map[string]any{nested, attributes} {
		if source == nil {
			continue
		}
		for _, key := range []string{"http.status_code", "http.response.status_code"} {
			if value := findInt(source, key); value > 0 {
				return value
			}
		}
	}

	return 0
}

func normalizeSpanResourceRoute(resourceName string) string {
	trimmed := strings.TrimSpace(resourceName)
	if trimmed == "" {
		return ""
	}

	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if method := strings.TrimSpace(parts[0]); method == "" || strings.ToUpper(method) != method {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func findString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	rawValue, ok := values[key]
	if !ok {
		return ""
	}

	switch typed := rawValue.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

func findInt(values map[string]any, key string) int {
	return int(findInt64(values, key))
}

func findInt64(values map[string]any, key string) int64 {
	if values == nil {
		return 0
	}
	rawValue, ok := values[key]
	if !ok {
		return 0
	}

	switch typed := rawValue.(type) {
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, typed)
		if err == nil {
			return parsed.UnixMilli()
		}
		var value int64
		if _, err := fmt.Sscanf(typed, "%d", &value); err != nil {
			return 0
		}
		return value
	default:
		return 0
	}
}

func indexErrorTrackingIncludedItems(items []searchIssueIncludedItem) (map[string]errorTrackingIssue, map[string]errorTrackingTeam) {
	issues := make(map[string]errorTrackingIssue)
	teams := make(map[string]errorTrackingTeam)

	for _, item := range items {
		switch normalizeErrorTrackingIncludedType(item.Type) {
		case "issue":
			var attributes errorTrackingIssueAttributes
			if err := json.Unmarshal(item.Attributes, &attributes); err != nil {
				continue
			}
			issues[item.ID] = errorTrackingIssue{
				ID:            item.ID,
				Attributes:    attributes,
				Relationships: item.Relationships,
			}
		case "team":
			var attributes teamAttributes
			if err := json.Unmarshal(item.Attributes, &attributes); err != nil {
				continue
			}
			teams[item.ID] = errorTrackingTeam{
				ID:     item.ID,
				Handle: attributes.Handle,
				Name:   attributes.Name,
			}
		}
	}

	return issues, teams
}

func normalizeErrorTrackingIncludedType(itemType string) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case "issue", "error_tracking_issue":
		return "issue"
	case "team", "error_tracking_team":
		return "team"
	default:
		return ""
	}
}

func errorTrackingTimestamp(attributes errorTrackingIssueAttributes) (time.Time, bool) {
	switch {
	case attributes.LastSeen > 0:
		return time.UnixMilli(attributes.LastSeen).UTC(), true
	case attributes.FirstSeen > 0:
		return time.UnixMilli(attributes.FirstSeen).UTC(), true
	default:
		return time.Time{}, false
	}
}

func mapErrorTrackingSeverity(state string, isCrash bool) domain.Severity {
	if isCrash {
		return domain.SeverityCritical
	}

	switch strings.ToLower(strings.TrimSpace(state)) {
	case "resolved":
		return domain.SeverityWarning
	case "ignored":
		return domain.SeverityInfo
	default:
		return domain.SeverityAlert
	}
}

func joinErrorTypeAndMessage(errorType string, message string) string {
	errorType = strings.TrimSpace(errorType)
	message = strings.TrimSpace(message)

	switch {
	case errorType != "" && message != "":
		return errorType + ": " + message
	case errorType != "":
		return errorType
	default:
		return message
	}
}

func resolveOwnerTeam(relationships issueIncludedRelationships, teams map[string]errorTrackingTeam) string {
	for _, owner := range relationships.TeamOwners.Data {
		team, ok := teams[owner.ID]
		if !ok {
			continue
		}
		if team.Handle != "" {
			return team.Handle
		}
		if team.Name != "" {
			return team.Name
		}
	}

	return ""
}

func formatUnixMilli(value int64) string {
	if value <= 0 {
		return ""
	}

	return time.UnixMilli(value).UTC().Format(time.RFC3339)
}

func appendErrorTrackingSamples(debug *outbound.AlertProviderDebug, issue errorTrackingIssue, queryMetadata queryMetadata, track string) {
	summary := strings.TrimSpace(issue.Attributes.ErrorType)
	if summary == "" {
		summary = strings.TrimSpace(issue.Attributes.ErrorMessage)
	}
	if summary != "" && len(debug.SampleMonitorNames) < 5 {
		debug.SampleMonitorNames = append(debug.SampleMonitorNames, summary)
	}

	candidates := []string{
		"service:" + issue.Attributes.Service,
		"env:" + queryMetadata.Environment,
		"state:" + issue.Attributes.State,
		"track:" + track,
	}
	for _, candidate := range candidates {
		if candidate == "service:" || candidate == "env:" || candidate == "state:" || candidate == "track:" {
			continue
		}
		if len(debug.SampleTags) >= 8 {
			return
		}
		if !slices.Contains(debug.SampleTags, candidate) {
			debug.SampleTags = append(debug.SampleTags, candidate)
		}
	}
}
