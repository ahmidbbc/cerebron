package datadog

import (
	"context"
	"strings"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

func (p AlertProvider) CollectSignals(ctx context.Context, query outbound.CollectSignalsQuery) ([]domain.Signal, error) {
	events, err := p.FetchAlerts(ctx, query.Since)
	if err != nil {
		return nil, err
	}

	return mapDatadogEventsToSignals(events, domain.SignalTypeMetric, query), nil
}

func (p EventAlertProvider) CollectSignals(ctx context.Context, query outbound.CollectSignalsQuery) ([]domain.Signal, error) {
	events, err := p.FetchAlerts(ctx, query.Since)
	if err != nil {
		return nil, err
	}

	return mapDatadogEventsToSignals(events, domain.SignalTypeMetric, query), nil
}

func (p ErrorTrackingProvider) CollectSignals(ctx context.Context, query outbound.CollectSignalsQuery) ([]domain.Signal, error) {
	service := firstService(query.Services)
	dynamicQuery := buildErrorTrackingQuery(p.query, service)
	events, err := p.fetchAlertsWithQuery(ctx, query.Since, dynamicQuery)
	if err != nil {
		return nil, err
	}

	return mapDatadogEventsToSignals(events, domain.SignalTypeLog, query), nil
}

func firstService(services []string) string {
	for _, s := range services {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

// buildErrorTrackingQuery prepends service:X to the base query when no service filter is already present.
func buildErrorTrackingQuery(baseQuery string, service string) string {
	if service == "" {
		return baseQuery
	}
	serviceFilter := "service:" + service
	if strings.Contains(baseQuery, "service:") {
		return baseQuery
	}
	if baseQuery == "" {
		return serviceFilter
	}
	return serviceFilter + " " + baseQuery
}

func mapDatadogEventsToSignals(events []domain.Event, signalType string, query outbound.CollectSignalsQuery) []domain.Signal {
	signals := make([]domain.Signal, 0, len(events))

	for _, event := range events {
		if !matchesAnySignalService(query.Services, event.Service) {
			continue
		}
		if !withinSignalWindow(event.OccurredAt, query.Since, query.Until) {
			continue
		}

		signals = append(signals, domain.Signal{
			Source:    domain.SignalSourceDatadog,
			Service:   event.Service,
			Type:      signalType,
			Summary:   signalSummaryFromEvent(event),
			Severity:  domain.SeverityToSignalSeverity(event.Severity),
			Timestamp: event.OccurredAt,
		})
	}

	return signals
}

func signalSummaryFromEvent(event domain.Event) string {
	for _, candidate := range []string{event.Summary, event.Message, event.Error} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}

	return event.Fingerprint
}

func matchesAnySignalService(services []string, actual string) bool {
	if len(services) == 0 {
		return true
	}
	actualLower := strings.ToLower(strings.TrimSpace(actual))
	for _, s := range services {
		if strings.ToLower(strings.TrimSpace(s)) == actualLower {
			return true
		}
	}
	return false
}

func withinSignalWindow(timestamp time.Time, since time.Time, until time.Time) bool {
	if !since.IsZero() && timestamp.Before(since) {
		return false
	}
	if !until.IsZero() && timestamp.After(until) {
		return false
	}

	return true
}
