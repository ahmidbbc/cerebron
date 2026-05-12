package analyzeincident

import (
	"context"
	"errors"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

type signalProviderStub struct {
	name    string
	signals []domain.Signal
	err     error
}

func (s signalProviderStub) Name() string {
	return s.name
}

func (s signalProviderStub) CollectSignals(context.Context, outbound.CollectSignalsQuery) ([]domain.Signal, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.signals, nil
}

func TestServiceRunBuildsIncidentAnalysis(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 27, 14, 10, 0, 0, time.UTC)
	service := NewService([]outbound.SignalProvider{
		signalProviderStub{
			name: "datadog",
			signals: []domain.Signal{
				{
					Source:    domain.SignalSourceDatadog,
					Service:   "catalog-api",
					Type:      domain.SignalTypeMetric,
					Summary:   "Latency alert",
					Severity:  domain.SignalSeverityHigh,
					Timestamp: time.Date(2026, 4, 27, 14, 2, 0, 0, time.UTC),
				},
			},
		},
		signalProviderStub{
			name: "elastic",
			signals: []domain.Signal{
				{
					Source:    domain.SignalSourceElastic,
					Service:   "catalog-api",
					Type:      domain.SignalTypeLog,
					Summary:   "Timeout spike",
					Severity:  domain.SignalSeverityMedium,
					Timestamp: time.Date(2026, 4, 27, 14, 4, 0, 0, time.UTC),
				},
				{
					Source:    domain.SignalSourceElastic,
					Service:   "catalog-api",
					Type:      domain.SignalTypeLog,
					Summary:   "Retry storm",
					Severity:  domain.SignalSeverityLow,
					Timestamp: time.Date(2026, 4, 27, 14, 7, 0, 0, time.UTC),
				},
			},
		},
	}, noopLogger())
	service.now = func() time.Time { return now }

	analysis, err := service.Run(context.Background(), Input{
		Services: []string{"catalog-api"},
		Since:   time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
		Until:   time.Date(2026, 4, 27, 14, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if analysis.Service != "catalog-api" {
		t.Fatalf("expected service catalog-api, got %q", analysis.Service)
	}
	if analysis.ModelVersion != domain.IncidentAnalysisModelVersion {
		t.Fatalf("expected model version %q, got %q", domain.IncidentAnalysisModelVersion, analysis.ModelVersion)
	}
	if analysis.Metadata.TotalSignals != 3 {
		t.Fatalf("expected 3 total signals, got %d", analysis.Metadata.TotalSignals)
	}
	if analysis.Metadata.DistinctSources != 2 {
		t.Fatalf("expected 2 distinct sources, got %d", analysis.Metadata.DistinctSources)
	}
	if len(analysis.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(analysis.Groups))
	}
	if analysis.Groups[0].HighestSeverity != domain.SignalSeverityHigh {
		t.Fatalf("expected first group highest severity high, got %q", analysis.Groups[0].HighestSeverity)
	}
	if analysis.Groups[0].SourceCount != 2 {
		t.Fatalf("expected first group source count 2, got %d", analysis.Groups[0].SourceCount)
	}
	if analysis.Confidence != 0.61 {
		t.Fatalf("expected confidence 0.61, got %0.2f", analysis.Confidence)
	}
}

func TestServiceRunFiltersSignalsByServiceAndWindow(t *testing.T) {
	t.Parallel()

	service := NewService([]outbound.SignalProvider{
		signalProviderStub{
			name: "datadog",
			signals: []domain.Signal{
				{
					Source:    domain.SignalSourceDatadog,
					Service:   "billing-api",
					Severity:  domain.SignalSeverityHigh,
					Timestamp: time.Date(2026, 4, 27, 14, 2, 0, 0, time.UTC),
				},
				{
					Source:    domain.SignalSourceDatadog,
					Service:   "catalog-api",
					Severity:  domain.SignalSeverityHigh,
					Timestamp: time.Date(2026, 4, 27, 13, 50, 0, 0, time.UTC),
				},
			},
		},
	}, noopLogger())

	analysis, err := service.Run(context.Background(), Input{
		Services: []string{"catalog-api"},
		Since:   time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
		Until:   time.Date(2026, 4, 27, 14, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if analysis.Metadata.TotalSignals != 0 {
		t.Fatalf("expected 0 total signals, got %d", analysis.Metadata.TotalSignals)
	}
	if len(analysis.Groups) != 0 {
		t.Fatalf("expected no groups, got %d", len(analysis.Groups))
	}
	if analysis.Confidence != 0 {
		t.Fatalf("expected confidence 0, got %v", analysis.Confidence)
	}
}

func TestServiceRunToleratesProviderFailure(t *testing.T) {
	t.Parallel()

	service := NewService([]outbound.SignalProvider{
		signalProviderStub{name: "elastic", err: errors.New("boom")},
	}, noopLogger())

	analysis, err := service.Run(context.Background(), Input{
		Services: []string{"catalog-api"},
		Since:    time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
		Until:    time.Date(2026, 4, 27, 14, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error on partial failure, got %v", err)
	}
	if analysis.Metadata.TotalSignals != 0 {
		t.Fatalf("expected zero signals when provider fails, got %d", analysis.Metadata.TotalSignals)
	}
}

func TestServiceRunPartialSuccessWhenOneProviderFails(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 27, 14, 10, 0, 0, time.UTC)
	service := NewService([]outbound.SignalProvider{
		signalProviderStub{name: "datadog", err: errors.New("datadog down")},
		signalProviderStub{
			name: "elastic",
			signals: []domain.Signal{
				{
					Source:    domain.SignalSourceElastic,
					Service:   "catalog-api",
					Type:      domain.SignalTypeLog,
					Summary:   "DB timeout",
					Severity:  domain.SignalSeverityHigh,
					Timestamp: time.Date(2026, 4, 27, 14, 5, 0, 0, time.UTC),
				},
			},
		},
	}, noopLogger())
	service.now = func() time.Time { return now }

	analysis, err := service.Run(context.Background(), Input{
		Services: []string{"catalog-api"},
		Since:    time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
		Until:    now,
	})
	if err != nil {
		t.Fatalf("expected no error on partial failure, got %v", err)
	}
	if analysis.Metadata.TotalSignals != 1 {
		t.Fatalf("expected 1 signal from surviving provider, got %d", analysis.Metadata.TotalSignals)
	}
}

func TestServiceRunRespectsProviderTimeout(t *testing.T) {
	t.Parallel()

	slow := &slowProviderStub{delay: 200 * time.Millisecond}
	service := NewService([]outbound.SignalProvider{slow}, noopLogger(),
		WithProviderTimeout(10*time.Millisecond),
	)

	analysis, err := service.Run(context.Background(), Input{
		Services: []string{"catalog-api"},
		Since:    time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
		Until:    time.Date(2026, 4, 27, 14, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error (timeout is a partial failure), got %v", err)
	}
	if analysis.Metadata.TotalSignals != 0 {
		t.Fatalf("expected zero signals on timeout, got %d", analysis.Metadata.TotalSignals)
	}
}

func TestServiceRunAcceptsEmptyServices(t *testing.T) {
	t.Parallel()

	service := NewService(nil, noopLogger())

	analysis, err := service.Run(context.Background(), Input{
		Since: time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 4, 27, 14, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if analysis.Confidence != 0 {
		t.Fatalf("expected zero confidence with no providers, got %v", analysis.Confidence)
	}
}
