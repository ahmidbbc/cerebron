package monitor

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
	reportingusecase "cerebron/internal/usecase/reporting"
)

type Input struct {
	Since        time.Time
	Until        time.Time
	Environments []string
	Debug        bool
}

type Result struct {
	AlertEvents          int
	LogEvents            int
	CollectedEvents      int
	CorrelatedEvents     int
	ObservedEnvironments []string
	Reportable           bool
	Score                int
	Debug                []ProviderDebug
	DebugEvents          []DebugEvent
	DebugCorrelations    []DebugCorrelation
}

const (
	ModeTest = "test"
	ModeProd = "prod"
)

type ProviderDebug struct {
	Provider              string
	MonitorsFetched       int
	CandidateEvents       int
	ReturnedByProvider    int
	FilteredByEnvironment int
	FilteredByTimeWindow  int
	IgnoredByStatus       int
	IgnoredBeforeSince    int
	SampleMonitorNames    []string
	SampleTags            []string
}

type DebugEvent struct {
	Source      string
	Service     string
	Environment string
	OccurredAt  time.Time
	StatusCode  int
	Route       string
	Message     string
	Error       string
}

type DebugCorrelation struct {
	Alert   DebugEvent
	Log     DebugEvent
	Score   int
	Reasons []string
}

type Service struct {
	alertProviders      []outbound.AlertProvider
	logProviders        []outbound.LogProvider
	reportingService    reportingusecase.Service
	defaultLookback     time.Duration
	environmentMode     string
	defaultEnvironments []string
	now                 func() time.Time
}

func NewService(
	alertProviders []outbound.AlertProvider,
	logProviders []outbound.LogProvider,
	reportingService reportingusecase.Service,
	defaultLookback time.Duration,
	environmentMode string,
	defaultEnvironments []string,
) Service {
	return Service{
		alertProviders:      alertProviders,
		logProviders:        logProviders,
		reportingService:    reportingService,
		defaultLookback:     defaultLookback,
		environmentMode:     normalizeMode(environmentMode),
		defaultEnvironments: slices.Clone(defaultEnvironments),
		now:                 time.Now,
	}
}

func (s Service) Run(ctx context.Context, input Input) (Result, error) {
	until := input.Until
	if until.IsZero() {
		until = s.now()
	}

	since := input.Since
	if since.IsZero() {
		since = until.Add(-s.defaultLookback)
	}

	if since.After(until) {
		return Result{}, fmt.Errorf("invalid monitoring window: since %s after until %s", since.Format(time.RFC3339), until.Format(time.RFC3339))
	}

	environments := input.Environments
	if len(environments) == 0 {
		environments = slices.Clone(s.defaultEnvironments)
	}

	events := make([]domain.Event, 0)
	alertEvents := 0
	logEvents := 0
	alertCandidates := make([]domain.Event, 0)
	debugEntries := make([]ProviderDebug, 0, len(s.alertProviders))
	policy := environmentPolicy{mode: s.environmentMode}

	for _, provider := range s.alertProviders {
		providerEvents, providerDebug, err := fetchAlertProviderEvents(ctx, provider, since)
		if err != nil {
			return Result{}, fmt.Errorf("fetch alerts from %s: %w", provider.Name(), err)
		}

		filteredEvents, filterStats := filterEvents(providerEvents, environments, policy, since, until)
		alertEvents += len(filteredEvents)
		alertCandidates = append(alertCandidates, filteredEvents...)
		events, _ = appendUniqueEvents(events, filteredEvents)
		if input.Debug {
			debugEntries = append(debugEntries, ProviderDebug{
				Provider:              provider.Name(),
				MonitorsFetched:       providerDebug.MonitorsFetched,
				CandidateEvents:       providerDebug.CandidateEvents,
				ReturnedByProvider:    len(providerEvents),
				FilteredByEnvironment: filterStats.FilteredByEnvironment,
				FilteredByTimeWindow:  filterStats.FilteredByTimeWindow,
				IgnoredByStatus:       providerDebug.IgnoredByStatus,
				IgnoredBeforeSince:    providerDebug.IgnoredBeforeSince,
				SampleMonitorNames:    providerDebug.SampleMonitorNames,
				SampleTags:            providerDebug.SampleTags,
			})
		}
	}

	for _, provider := range s.logProviders {
		logEnvironments := environments
		if len(logEnvironments) == 0 {
			logEnvironments = []string{""}
		}
		for _, environment := range logEnvironments {
			providerEvents, err := provider.SearchLogs(ctx, domain.LogQuery{
				Environment: environment,
				Since:       since,
				Until:       until,
			})
			if err != nil {
				return Result{}, fmt.Errorf("search logs from %s for environment %s: %w", provider.Name(), environment, err)
			}

			filteredEvents, _ := filterEvents(providerEvents, environments, policy, since, until)
			var added int
			events, added = appendUniqueEvents(events, filteredEvents)
			logEvents += added
		}
	}

	for _, provider := range s.logProviders {
		targetedQueries := buildTargetedLogQueries(alertCandidates, since, until)
		for _, query := range targetedQueries {
			providerEvents, err := provider.SearchLogs(ctx, query)
			if err != nil {
				return Result{}, fmt.Errorf("search targeted logs from %s for service %s: %w", provider.Name(), query.Service, err)
			}

			filteredEvents, _ := filterEvents(providerEvents, environments, policy, since, until)
			var added int
			events, added = appendUniqueEvents(events, filteredEvents)
			logEvents += added
		}
	}

	correlatedEvents, debugCorrelations := correlateEvents(events, input.Debug)

	return Result{
		AlertEvents:          alertEvents,
		LogEvents:            logEvents,
		CollectedEvents:      len(events),
		CorrelatedEvents:     correlatedEvents,
		ObservedEnvironments: environments,
		Reportable:           s.reportingService.ShouldPublish(events),
		Score:                s.reportingService.Score(events),
		Debug:                debugEntries,
		DebugEvents:          sampleDebugEvents(events, input.Debug),
		DebugCorrelations:    debugCorrelations,
	}, nil
}

func buildTargetedLogQueries(alertEvents []domain.Event, since time.Time, until time.Time) []domain.LogQuery {
	queries := make([]domain.LogQuery, 0, len(alertEvents))
	seen := make(map[string]struct{})

	for _, event := range alertEvents {
		if strings.TrimSpace(event.Service) == "" {
			continue
		}

		baseQuery := domain.LogQuery{
			Service:     event.Service,
			Environment: event.Environment,
			Since:       since,
			Until:       until,
		}

		queries = appendQueryIfNew(queries, seen, baseQuery)

		terms := correlationTerms(event)
		if len(terms) == 0 {
			continue
		}

		queries = appendQueryIfNew(queries, seen, domain.LogQuery{
			Service:     event.Service,
			Environment: event.Environment,
			Since:       since,
			Until:       until,
			Terms:       terms,
		})
	}

	return queries
}

func appendQueryIfNew(queries []domain.LogQuery, seen map[string]struct{}, query domain.LogQuery) []domain.LogQuery {
	key := query.Environment + "|" + query.Service + "|" + strings.Join(query.Terms, "|")
	if _, ok := seen[key]; ok {
		return queries
	}
	seen[key] = struct{}{}

	return append(queries, query)
}

func correlationTerms(event domain.Event) []string {
	terms := make([]string, 0, 4)
	for _, candidate := range []string{event.Error, event.Message} {
		trimmed := strings.TrimSpace(candidate)
		if len(trimmed) < 8 {
			continue
		}
		if slices.Contains(terms, trimmed) {
			continue
		}
		terms = append(terms, trimmed)
	}

	return terms
}

func appendUniqueEvents(existing []domain.Event, additions []domain.Event) ([]domain.Event, int) {
	if len(additions) == 0 {
		return existing, 0
	}

	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, event := range existing {
		seen[eventKey(event)] = struct{}{}
	}

	added := 0
	for _, event := range additions {
		key := eventKey(event)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, event)
		added++
	}

	return existing, added
}

func eventKey(event domain.Event) string {
	if event.ID != "" {
		return string(event.Source) + ":" + event.ID
	}
	if event.Fingerprint != "" {
		return string(event.Source) + ":" + event.Fingerprint
	}

	return string(event.Source) + ":" + event.Service + ":" + event.Environment + ":" + event.OccurredAt.UTC().Format(time.RFC3339Nano)
}

func sampleDebugEvents(events []domain.Event, enabled bool) []DebugEvent {
	if !enabled {
		return nil
	}

	samples := make([]DebugEvent, 0, minInt(len(events), 8))
	for _, event := range events {
		if len(samples) >= 8 {
			break
		}
		samples = append(samples, debugEventFromDomainEvent(event))
	}

	return samples
}

func debugEventFromDomainEvent(event domain.Event) DebugEvent {
	return DebugEvent{
		Source:      string(event.Source),
		Service:     event.Service,
		Environment: event.Environment,
		OccurredAt:  event.OccurredAt,
		StatusCode:  event.StatusCode,
		Route:       event.Route,
		Message:     event.Message,
		Error:       event.Error,
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}

	return right
}

type filterDebug struct {
	FilteredByEnvironment int
	FilteredByTimeWindow  int
}

func fetchAlertProviderEvents(ctx context.Context, provider outbound.AlertProvider, since time.Time) ([]domain.Event, outbound.AlertProviderDebug, error) {
	if debugProvider, ok := provider.(outbound.DebugAlertProvider); ok {
		result, err := debugProvider.FetchAlertsDebug(ctx, since)
		if err != nil {
			return nil, outbound.AlertProviderDebug{}, err
		}

		return result.Events, result.Debug, nil
	}

	events, err := provider.FetchAlerts(ctx, since)
	if err != nil {
		return nil, outbound.AlertProviderDebug{}, err
	}

	return events, outbound.AlertProviderDebug{}, nil
}

type environmentPolicy struct {
	mode string
}

func (p environmentPolicy) allows(environment string) bool {
	switch normalizeMode(p.mode) {
	case ModeProd:
		return environmentFamily(environment) == "prod"
	default:
		return environmentFamily(environment) != "prod"
	}
}

func filterEvents(events []domain.Event, environments []string, policy environmentPolicy, since time.Time, until time.Time) ([]domain.Event, filterDebug) {
	filteredEvents := make([]domain.Event, 0, len(events))
	debug := filterDebug{}

	for _, event := range events {
		if event.OccurredAt.Before(since) || event.OccurredAt.After(until) {
			debug.FilteredByTimeWindow++
			continue
		}
		if len(environments) > 0 {
			if event.Environment == "" || !matchesEnvironmentFilters(environments, event.Environment) {
				debug.FilteredByEnvironment++
				continue
			}
		} else if !policy.allows(event.Environment) {
			debug.FilteredByEnvironment++
			continue
		}

		filteredEvents = append(filteredEvents, event)
	}

	return filteredEvents, debug
}

func matchesEnvironmentFilters(filters []string, environment string) bool {
	for _, filter := range filters {
		if environmentMatchesFilter(filter, environment) {
			return true
		}
	}

	return false
}

const correlationWindow = 30 * time.Minute

func correlateEvents(events []domain.Event, debugEnabled bool) (int, []DebugCorrelation) {
	matches := 0
	debugCorrelations := make([]DebugCorrelation, 0, 8)
	seenGroups := make(map[string]struct{})

	for i := range events {
		if events[i].Source != domain.SourceElasticsearch {
			continue
		}

		for j := range events {
			if i == j {
				continue
			}
			if events[j].Source != domain.SourceDatadog {
				continue
			}
			score, reasons, ok := correlationScore(events[j], events[i])
			if !ok {
				continue
			}

			groupKey := correlationGroupKey(events[j], events[i])
			if _, seen := seenGroups[groupKey]; seen {
				continue
			}
			seenGroups[groupKey] = struct{}{}

			if !slices.Contains(events[i].CorrelatedIDs, events[j].ID) {
				events[i].CorrelatedIDs = append(events[i].CorrelatedIDs, events[j].ID)
				matches++
				if debugEnabled && len(debugCorrelations) < 8 {
					debugCorrelations = append(debugCorrelations, DebugCorrelation{
						Alert:   debugEventFromDomainEvent(events[j]),
						Log:     debugEventFromDomainEvent(events[i]),
						Score:   score,
						Reasons: reasons,
					})
				}
			}
			if !slices.Contains(events[j].CorrelatedIDs, events[i].ID) {
				events[j].CorrelatedIDs = append(events[j].CorrelatedIDs, events[i].ID)
			}
		}
	}

	return matches, debugCorrelations
}

func correlationScore(alertEvent, logEvent domain.Event) (int, []string, bool) {
	if !sameService(alertEvent.Service, logEvent.Service) {
		return 0, nil, false
	}
	if !sameEnvironment(alertEvent.Environment, logEvent.Environment) {
		return 0, nil, false
	}

	timeDelta := alertEvent.OccurredAt.Sub(logEvent.OccurredAt)
	if timeDelta < 0 {
		timeDelta = -timeDelta
	}
	if timeDelta > correlationWindow {
		return 0, nil, false
	}

	score := 0
	reasons := []string{"same_service", "same_environment_family"}
	if timeDelta <= 5*time.Minute {
		score += 3
		reasons = append(reasons, "time_delta_lte_5m")
	} else {
		score++
		reasons = append(reasons, "time_delta_lte_30m")
	}

	score += 3

	hasSameStatus := sameStatus(alertEvent, logEvent)
	hasSameRoute := sameRoute(alertEvent.Route, logEvent.Route)
	hasSameErrorSignature := sameErrorSignature(alertEvent, logEvent)

	if requiresSupportingSignal(alertEvent) && !(hasSameStatus || hasSameRoute || hasSameErrorSignature) {
		return 0, nil, false
	}

	if hasSameStatus {
		score += 3
		reasons = append(reasons, "same_status")
	}
	if hasSameRoute {
		score++
		reasons = append(reasons, "same_route")
	}
	if hasSameErrorSignature {
		score += 2
		reasons = append(reasons, "same_error_signature")
	}

	return score, reasons, score >= 6
}

func requiresSupportingSignal(alertEvent domain.Event) bool {
	if alertEvent.Attributes == nil {
		return false
	}

	return strings.TrimSpace(alertEvent.Attributes["issue_id"]) != "" && strings.TrimSpace(alertEvent.Attributes["track"]) != ""
}

func correlationGroupKey(alertEvent, logEvent domain.Event) string {
	return strings.Join([]string{
		eventKey(alertEvent),
		strings.ToLower(strings.TrimSpace(logEvent.Service)),
		environmentFamily(logEvent.Environment),
		normalizeCorrelationGroupValue(logEvent.Route),
		logCorrelationStatus(logEvent),
		logEvent.OccurredAt.UTC().Truncate(time.Minute).Format(time.RFC3339),
	}, "|")
}

func logCorrelationStatus(event domain.Event) string {
	if event.StatusCode > 0 {
		return strconv.Itoa(event.StatusCode)
	}
	if strings.TrimSpace(event.StatusClass) != "" {
		return strings.TrimSpace(event.StatusClass)
	}

	return "-"
}

func normalizeCorrelationGroupValue(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "-"
	}

	return trimmed
}

func sameService(left, right string) bool {
	left = strings.TrimSpace(strings.ToLower(left))
	right = strings.TrimSpace(strings.ToLower(right))

	return left != "" && right != "" && left == right
}

func sameEnvironment(left, right string) bool {
	if left == "" || right == "" {
		return true
	}

	return environmentFamily(left) == environmentFamily(right)
}

func sameStatus(left, right domain.Event) bool {
	switch {
	case left.StatusCode > 0 && right.StatusCode > 0:
		return left.StatusCode == right.StatusCode
	case left.StatusClass != "" && right.StatusClass != "":
		return left.StatusClass == right.StatusClass
	case left.StatusCode > 0 && right.StatusClass != "":
		return statusClassFromCode(left.StatusCode) == right.StatusClass
	case left.StatusClass != "" && right.StatusCode > 0:
		return left.StatusClass == statusClassFromCode(right.StatusCode)
	default:
		return false
	}
}

func sameRoute(left, right string) bool {
	left = normalizeRoute(left)
	right = normalizeRoute(right)

	if left == "" || right == "" {
		return false
	}

	return left == right || strings.HasPrefix(left, right) || strings.HasPrefix(right, left)
}

func sameErrorSignature(alertEvent, logEvent domain.Event) bool {
	alertCandidates := []string{
		normalizeErrorText(alertEvent.Error),
		normalizeErrorText(alertEvent.Message),
	}
	logCandidates := []string{
		normalizeErrorText(logEvent.Error),
		normalizeErrorText(logEvent.Message),
	}

	for _, alertCandidate := range alertCandidates {
		if alertCandidate == "" {
			continue
		}
		for _, logCandidate := range logCandidates {
			if logCandidate == "" {
				continue
			}
			if errorTextsMatch(alertCandidate, logCandidate) {
				return true
			}
		}
	}

	return false
}

func normalizeErrorText(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return ""
	}

	return normalized
}

func errorTextsMatch(left, right string) bool {
	if len(left) < 8 || len(right) < 8 {
		return false
	}

	return strings.Contains(left, right) || strings.Contains(right, left)
}

func normalizeRoute(route string) string {
	trimmed := strings.TrimSpace(strings.ToLower(route))
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return ""
	}

	return trimmed
}

func statusClassFromCode(statusCode int) string {
	if statusCode < 100 || statusCode > 599 {
		return ""
	}

	return fmt.Sprintf("%dxx", statusCode/100)
}

func environmentMatchesFilter(filter string, environment string) bool {
	filterFamily := environmentFamily(filter)
	environmentKind := environmentFamily(environment)

	switch filterFamily {
	case "prod", "qa", "preprod":
		return environmentKind == filterFamily
	default:
		return strings.EqualFold(strings.TrimSpace(filter), strings.TrimSpace(environment))
	}
}

func environmentFamily(environment string) string {
	normalized := strings.ToLower(strings.TrimSpace(environment))

	switch {
	case normalized == "":
		return "unknown"
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

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeProd:
		return ModeProd
	default:
		return ModeTest
	}
}
