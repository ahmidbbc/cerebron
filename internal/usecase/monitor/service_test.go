package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
	"cerebron/internal/usecase/reporting"
)

type alertProviderStub struct {
	name   string
	events []domain.Event
	err    error
}

func (s alertProviderStub) Name() string {
	return s.name
}

func (s alertProviderStub) FetchAlerts(context.Context, time.Time) ([]domain.Event, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.events, nil
}

type debugAlertProviderStub struct {
	name   string
	result outbound.AlertFetchResult
	err    error
}

func (s debugAlertProviderStub) Name() string {
	return s.name
}

func (s debugAlertProviderStub) FetchAlerts(context.Context, time.Time) ([]domain.Event, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.result.Events, nil
}

func (s debugAlertProviderStub) FetchAlertsDebug(context.Context, time.Time) (outbound.AlertFetchResult, error) {
	if s.err != nil {
		return outbound.AlertFetchResult{}, s.err
	}

	return s.result, nil
}

type logProviderStub struct {
	name   string
	events []domain.Event
	err    error
}

func (s logProviderStub) Name() string {
	return s.name
}

func (s logProviderStub) SearchLogs(context.Context, domain.LogQuery) ([]domain.Event, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.events, nil
}

type targetedLogProviderStub struct {
	name   string
	search func(domain.LogQuery) ([]domain.Event, error)
}

func (s targetedLogProviderStub) Name() string {
	return s.name
}

func (s targetedLogProviderStub) SearchLogs(_ context.Context, query domain.LogQuery) ([]domain.Event, error) {
	return s.search(query)
}

func TestServiceRunCollectsAlertsAndLogs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog",
			events: []domain.Event{{
				Environment: "preprod",
				Severity:    domain.SeverityAlert,
				OccurredAt:  now.Add(-2 * time.Minute),
			}},
		}},
		[]outbound.LogProvider{logProviderStub{
			name: "elastic",
			events: []domain.Event{{
				Environment: "preprod",
				Severity:    domain.SeverityWarning,
				OccurredAt:  now.Add(-1 * time.Minute),
			}},
		}},
		reporting.NewService(reporting.DefaultPolicy()),
		15*time.Minute,
		ModeTest,
		[]string{"preprod"},
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.AlertEvents != 1 {
		t.Fatalf("expected 1 alert event, got %d", result.AlertEvents)
	}
	if result.LogEvents != 1 {
		t.Fatalf("expected 1 log event, got %d", result.LogEvents)
	}
	if result.CollectedEvents != 2 {
		t.Fatalf("expected 2 collected events, got %d", result.CollectedEvents)
	}
	if !result.Reportable {
		t.Fatalf("expected collected events to be reportable")
	}
}

func TestServiceRunCorrelatesDatadogAndElasticEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 2, 17, 15, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog",
			events: []domain.Event{{
				ID:          "dd-1",
				Source:      domain.SourceDatadog,
				Service:     "import-ads-booking-private-api",
				Environment: "prod",
				Severity:    domain.SeverityAlert,
				StatusCode:  409,
				Route:       "/api/import-ads-booking/v1/bookings/consume",
				OccurredAt:  now.Add(-5 * time.Minute),
			}},
		}},
		[]outbound.LogProvider{logProviderStub{
			name: "elastic",
			events: []domain.Event{{
				ID:          "es-1",
				Source:      domain.SourceElasticsearch,
				Service:     "import-ads-booking-private-api",
				Environment: "prod-morpheus",
				Severity:    domain.SeverityWarning,
				StatusCode:  409,
				Route:       "/api/import-ads-booking/v1/bookings",
				OccurredAt:  now.Add(-4 * time.Minute),
			}},
		}},
		reporting.NewService(reporting.DefaultPolicy()),
		30*time.Minute,
		ModeProd,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlertEvents != 1 {
		t.Fatalf("expected 1 alert event, got %d", result.AlertEvents)
	}
	if result.LogEvents != 1 {
		t.Fatalf("expected 1 log event, got %d", result.LogEvents)
	}
	if result.CorrelatedEvents != 1 {
		t.Fatalf("expected 1 correlated event, got %d", result.CorrelatedEvents)
	}
	if result.Score < 100 {
		t.Fatalf("expected correlated events to increase score, got %d", result.Score)
	}
	if !result.Reportable {
		t.Fatalf("expected correlated events to be reportable")
	}
}

func TestServiceRunCorrelatesErrorTrackingAndElasticOnErrorMessage(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog-error-tracking",
			events: []domain.Event{{
				ID:          "dd-et-1",
				Source:      domain.SourceDatadog,
				Service:     "presence-api",
				Environment: "preprod",
				Severity:    domain.SeverityAlert,
				Message:     "illegal base64 data at input byte 0",
				Error:       "base64.CorruptInputError: illegal base64 data at input byte 0",
				OccurredAt:  now.Add(-20 * time.Minute),
			}},
		}},
		[]outbound.LogProvider{logProviderStub{
			name: "elastic",
			events: []domain.Event{{
				ID:          "es-identity-1",
				Source:      domain.SourceElasticsearch,
				Service:     "presence-api",
				Environment: "preprod-identity",
				Severity:    domain.SeverityWarning,
				StatusCode:  401,
				Message:     "can't read access token: illegal base64 data at input byte 0",
				OccurredAt:  now.Add(-2 * time.Minute),
			}},
		}},
		reporting.NewService(reporting.DefaultPolicy()),
		6*time.Hour,
		ModeTest,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlertEvents != 1 {
		t.Fatalf("expected 1 alert event, got %d", result.AlertEvents)
	}
	if result.LogEvents != 1 {
		t.Fatalf("expected 1 log event, got %d", result.LogEvents)
	}
	if result.CorrelatedEvents != 1 {
		t.Fatalf("expected 1 correlated event, got %d", result.CorrelatedEvents)
	}
}

func TestServiceRunRejectsErrorTrackingFalsePositiveWithoutSupportingSignal(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 7, 15, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog-error-tracking",
			events: []domain.Event{{
				ID:          "dd-et-1",
				Source:      domain.SourceDatadog,
				Service:     "presence-api",
				Environment: "preprod",
				Severity:    domain.SeverityAlert,
				StatusCode:  401,
				Route:       "/api/presence/me/status",
				Message:     "illegal base64 data at input byte 0",
				Error:       "base64.CorruptInputError: illegal base64 data at input byte 0",
				OccurredAt:  time.Date(2026, 4, 7, 14, 51, 5, 0, time.UTC),
				Attributes: map[string]string{
					"issue_id": "issue-1",
					"track":    "trace",
				},
			}},
		}},
		[]outbound.LogProvider{logProviderStub{
			name: "elastic",
			events: []domain.Event{
				{
					ID:          "es-1",
					Source:      domain.SourceElasticsearch,
					Service:     "presence-api",
					Environment: "preprod-identity",
					Severity:    domain.SeverityWarning,
					StatusCode:  401,
					Route:       "/api/presence/me/status",
					OccurredAt:  time.Date(2026, 4, 7, 14, 51, 5, 0, time.UTC),
				},
				{
					ID:          "es-2",
					Source:      domain.SourceElasticsearch,
					Service:     "presence-api",
					Environment: "preprod-identity",
					Severity:    domain.SeverityWarning,
					Message:     "failed to get last activity for presence",
					Error:       "context canceled",
					OccurredAt:  time.Date(2026, 4, 7, 14, 49, 36, 0, time.UTC),
				},
			},
		}},
		reporting.NewService(reporting.DefaultPolicy()),
		24*time.Hour,
		ModeTest,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{Debug: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.CorrelatedEvents != 1 {
		t.Fatalf("expected only the supporting-signal log to correlate, got %d", result.CorrelatedEvents)
	}
	if len(result.DebugCorrelations) != 1 {
		t.Fatalf("expected 1 debug correlation, got %d", len(result.DebugCorrelations))
	}
}

func TestServiceRunPerformsTargetedLogSearchFromAlertMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC)
	callCount := 0
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog-error-tracking",
			events: []domain.Event{{
				ID:          "dd-et-1",
				Source:      domain.SourceDatadog,
				Service:     "presence-api",
				Environment: "preprod",
				Severity:    domain.SeverityAlert,
				Message:     "illegal base64 data at input byte 0",
				Error:       "base64.CorruptInputError: illegal base64 data at input byte 0",
				OccurredAt:  now.Add(-20 * time.Minute),
			}},
		}},
		[]outbound.LogProvider{targetedLogProviderStub{
			name: "elastic",
			search: func(query domain.LogQuery) ([]domain.Event, error) {
				callCount++
				if query.Service == "" {
					return nil, nil
				}
				if query.Service != "presence-api" {
					t.Fatalf("expected targeted query for presence-api, got %q", query.Service)
				}
				if query.Environment != "preprod" {
					t.Fatalf("expected targeted query for preprod, got %q", query.Environment)
				}
				if len(query.Terms) == 0 {
					return []domain.Event{{
						ID:          "es-identity-2",
						Source:      domain.SourceElasticsearch,
						Service:     "presence-api",
						Environment: "preprod-identity",
						Severity:    domain.SeverityWarning,
						StatusCode:  401,
						OccurredAt:  now.Add(-2 * time.Minute),
					}}, nil
				}

				return []domain.Event{{
					ID:          "es-identity-3",
					Source:      domain.SourceElasticsearch,
					Service:     "presence-api",
					Environment: "preprod-identity",
					Severity:    domain.SeverityWarning,
					StatusCode:  401,
					Message:     "can't read access token: illegal base64 data at input byte 0",
					OccurredAt:  now.Add(-2 * time.Minute),
				}}, nil
			},
		}},
		reporting.NewService(reporting.DefaultPolicy()),
		6*time.Hour,
		ModeTest,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.CorrelatedEvents != 1 {
		t.Fatalf("expected targeted log search to yield 1 correlated event, got %d", result.CorrelatedEvents)
	}
	if callCount < 2 {
		t.Fatalf("expected targeted log search to try both service-only and term-filtered queries, got %d calls", callCount)
	}
}

func TestServiceRunDeduplicatesBurstCorrelations(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 7, 12, 42, 30, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog-error-tracking",
			events: []domain.Event{{
				ID:          "dd-et-1",
				Source:      domain.SourceDatadog,
				Service:     "presence-api",
				Environment: "preprod",
				Severity:    domain.SeverityAlert,
				Message:     "illegal base64 data at input byte 0",
				Error:       "base64.CorruptInputError: illegal base64 data at input byte 0",
				OccurredAt:  time.Date(2026, 4, 7, 12, 41, 58, 0, time.UTC),
			}},
		}},
		[]outbound.LogProvider{logProviderStub{
			name: "elastic",
			events: []domain.Event{
				{
					ID:          "es-1",
					Source:      domain.SourceElasticsearch,
					Service:     "presence-api",
					Environment: "preprod-identity",
					Severity:    domain.SeverityWarning,
					StatusCode:  401,
					Route:       "/api/presence/me/status",
					OccurredAt:  time.Date(2026, 4, 7, 12, 41, 58, 0, time.UTC),
				},
				{
					ID:          "es-2",
					Source:      domain.SourceElasticsearch,
					Service:     "presence-api",
					Environment: "preprod-identity",
					Severity:    domain.SeverityWarning,
					StatusCode:  401,
					Route:       "/api/presence/me/status",
					OccurredAt:  time.Date(2026, 4, 7, 12, 41, 49, 0, time.UTC),
				},
				{
					ID:          "es-3",
					Source:      domain.SourceElasticsearch,
					Service:     "presence-api",
					Environment: "preprod-identity",
					Severity:    domain.SeverityWarning,
					StatusCode:  401,
					Route:       "/api/presence/me/status",
					OccurredAt:  time.Date(2026, 4, 7, 12, 41, 40, 0, time.UTC),
				},
			},
		}},
		reporting.NewService(reporting.DefaultPolicy()),
		24*time.Hour,
		ModeTest,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{Debug: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.CorrelatedEvents != 1 {
		t.Fatalf("expected burst correlations to be deduplicated to 1, got %d", result.CorrelatedEvents)
	}
	if len(result.DebugCorrelations) != 1 {
		t.Fatalf("expected 1 debug correlation after deduplication, got %d", len(result.DebugCorrelations))
	}
}

func TestServiceRunReturnsProviderError(t *testing.T) {
	t.Parallel()

	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog",
			err:  errors.New("provider unavailable"),
		}},
		nil,
		reporting.NewService(reporting.DefaultPolicy()),
		15*time.Minute,
		ModeTest,
		[]string{"qa"},
	)

	_, err := service.Run(context.Background(), Input{})
	if err == nil {
		t.Fatalf("expected provider error")
	}
}

func TestServiceRunCollectsDebugDataWhenRequested(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{debugAlertProviderStub{
			name: "datadog",
			result: outbound.AlertFetchResult{
				Events: []domain.Event{{
					Environment: "qa",
					Severity:    domain.SeverityWarning,
					OccurredAt:  now.Add(-2 * time.Minute),
				}},
				Debug: outbound.AlertProviderDebug{
					MonitorsFetched:    3,
					CandidateEvents:    2,
					IgnoredByStatus:    1,
					IgnoredBeforeSince: 0,
				},
			},
		}},
		nil,
		reporting.NewService(reporting.DefaultPolicy()),
		15*time.Minute,
		ModeTest,
		[]string{"preprod"},
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{Debug: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Debug) != 1 {
		t.Fatalf("expected 1 debug entry, got %d", len(result.Debug))
	}
	if result.Debug[0].MonitorsFetched != 3 {
		t.Fatalf("expected 3 monitors fetched, got %d", result.Debug[0].MonitorsFetched)
	}
	if result.Debug[0].FilteredByEnvironment != 1 {
		t.Fatalf("expected 1 event filtered by environment, got %d", result.Debug[0].FilteredByEnvironment)
	}
}

func TestServiceRunMatchesQAEnvironmentFamily(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog",
			events: []domain.Event{{
				Environment: "qa3",
				Severity:    domain.SeverityAlert,
				OccurredAt:  now.Add(-2 * time.Minute),
			}},
		}},
		nil,
		reporting.NewService(reporting.DefaultPolicy()),
		15*time.Minute,
		ModeTest,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{
		Environments: []string{"qa"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlertEvents != 1 {
		t.Fatalf("expected qa filter to include qa3 event, got %d alert events", result.AlertEvents)
	}
	if len(result.ObservedEnvironments) != 1 || result.ObservedEnvironments[0] != "qa" {
		t.Fatalf("expected observed environments to keep explicit filter, got %v", result.ObservedEnvironments)
	}
}

func TestServiceRunMatchesPreprodEnvironmentFamily(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog",
			events: []domain.Event{{
				Environment: "preprod2",
				Severity:    domain.SeverityAlert,
				OccurredAt:  now.Add(-2 * time.Minute),
			}},
		}},
		nil,
		reporting.NewService(reporting.DefaultPolicy()),
		15*time.Minute,
		ModeTest,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{
		Environments: []string{"preprod"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlertEvents != 1 {
		t.Fatalf("expected preprod filter to include preprod2 event, got %d alert events", result.AlertEvents)
	}
}

func TestServiceRunTestModeExcludesProdByDefault(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog",
			events: []domain.Event{
				{
					Environment: "qa3",
					Severity:    domain.SeverityAlert,
					OccurredAt:  now.Add(-3 * time.Minute),
				},
				{
					Environment: "production",
					Severity:    domain.SeverityAlert,
					OccurredAt:  now.Add(-2 * time.Minute),
				},
				{
					Environment: "",
					Severity:    domain.SeverityWarning,
					OccurredAt:  now.Add(-1 * time.Minute),
				},
			},
		}},
		nil,
		reporting.NewService(reporting.DefaultPolicy()),
		15*time.Minute,
		ModeTest,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlertEvents != 2 {
		t.Fatalf("expected test mode to keep non-prod and unknown environments, got %d alert events", result.AlertEvents)
	}
	if len(result.ObservedEnvironments) != 0 {
		t.Fatalf("expected no explicit observed environments in default mode, got %v", result.ObservedEnvironments)
	}
}

func TestServiceRunProdModeKeepsOnlyProdByDefault(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	service := NewService(
		[]outbound.AlertProvider{alertProviderStub{
			name: "datadog",
			events: []domain.Event{
				{
					Environment: "prod",
					Severity:    domain.SeverityAlert,
					OccurredAt:  now.Add(-3 * time.Minute),
				},
				{
					Environment: "production-eu",
					Severity:    domain.SeverityAlert,
					OccurredAt:  now.Add(-2 * time.Minute),
				},
				{
					Environment: "preprod",
					Severity:    domain.SeverityAlert,
					OccurredAt:  now.Add(-1 * time.Minute),
				},
			},
		}},
		nil,
		reporting.NewService(reporting.DefaultPolicy()),
		15*time.Minute,
		ModeProd,
		nil,
	)
	service.now = func() time.Time { return now }

	result, err := service.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlertEvents != 2 {
		t.Fatalf("expected prod mode to keep only prod environments, got %d alert events", result.AlertEvents)
	}
}
