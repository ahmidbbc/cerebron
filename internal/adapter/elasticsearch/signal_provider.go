package elasticsearch

import (
	"context"
	"strings"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

func (p LogProvider) CollectSignals(ctx context.Context, query outbound.CollectSignalsQuery) ([]domain.Signal, error) {
	events, err := p.SearchLogs(ctx, domain.LogQuery{
		Service: firstService(query.Services),
		Since:   query.Since,
		Until:   query.Until,
	})
	if err != nil {
		return nil, err
	}

	signals := make([]domain.Signal, 0, len(events))
	for _, event := range events {
		if !matchesAnySignalService(query.Services, event.Service) {
			continue
		}
		if !withinSignalWindow(event.OccurredAt, query.Since, query.Until) {
			continue
		}

		signals = append(signals, domain.Signal{
			Source:    domain.SignalSourceElastic,
			Service:   event.Service,
			Type:      domain.SignalTypeLog,
			Summary:   signalSummaryFromEvent(event),
			Severity:  domain.SeverityToSignalSeverity(event.Severity),
			Timestamp: event.OccurredAt,
		})
	}

	return signals, nil
}

func firstService(services []string) string {
	for _, s := range services {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
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

func signalSummaryFromEvent(event domain.Event) string {
	for _, candidate := range []string{event.Summary, event.Message, event.Error} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}

	return event.Fingerprint
}
