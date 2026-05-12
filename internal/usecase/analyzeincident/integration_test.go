package analyzeincident

import (
	"context"
	"strings"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

// TestIntegrationFullFlow exercises the complete providers → grouping → IncidentAnalysis path.
// It uses two stubbed providers (one Datadog, one Elastic) producing signals across two
// 5-minute windows and asserts every required field in the output shape.
func TestIntegrationFullFlow(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	until := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)

	// Window 1: 10:00–10:05 — two signals from two sources
	// Window 2: 10:10–10:15 — one signal from one source
	ddSignals := []domain.Signal{
		{
			Source:    domain.SignalSourceDatadog,
			Service:   "payments",
			Type:      domain.SignalTypeMetric,
			Summary:   "Latency p99 alert",
			Severity:  domain.SignalSeverityHigh,
			Timestamp: time.Date(2026, 5, 1, 10, 2, 0, 0, time.UTC),
		},
		{
			Source:    domain.SignalSourceDatadog,
			Service:   "payments",
			Type:      domain.SignalTypeMetric,
			Summary:   "Error rate spike",
			Severity:  domain.SignalSeverityMedium,
			Timestamp: time.Date(2026, 5, 1, 10, 12, 0, 0, time.UTC),
		},
	}
	elasticSignals := []domain.Signal{
		{
			Source:    domain.SignalSourceElastic,
			Service:   "payments",
			Type:      domain.SignalTypeLog,
			Summary:   "NullPointerException burst",
			Severity:  domain.SignalSeverityHigh,
			Timestamp: time.Date(2026, 5, 1, 10, 3, 0, 0, time.UTC),
		},
	}

	svc := NewService([]outbound.SignalProvider{
		signalProviderStub{name: "datadog", signals: ddSignals},
		signalProviderStub{name: "elastic", signals: elasticSignals},
	}, noopLogger())
	svc.now = func() time.Time { return until }

	analysis, err := svc.Run(context.Background(), Input{
		Services: []string{"payments"},
		Since:   since,
		Until:   until,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// --- Required output fields ---

	if analysis.Service != "payments" {
		t.Errorf("Service: want %q, got %q", "payments", analysis.Service)
	}
	if analysis.ModelVersion != domain.IncidentAnalysisModelVersion {
		t.Errorf("ModelVersion: want %q, got %q", domain.IncidentAnalysisModelVersion, analysis.ModelVersion)
	}

	expectedTimeRange := since.UTC().Format(time.RFC3339) + "/" + until.UTC().Format(time.RFC3339)
	if analysis.TimeRange != expectedTimeRange {
		t.Errorf("TimeRange: want %q, got %q", expectedTimeRange, analysis.TimeRange)
	}

	// --- Metadata ---

	if analysis.Metadata.TotalSignals != 3 {
		t.Errorf("Metadata.TotalSignals: want 3, got %d", analysis.Metadata.TotalSignals)
	}
	if analysis.Metadata.DistinctSources != 2 {
		t.Errorf("Metadata.DistinctSources: want 2, got %d", analysis.Metadata.DistinctSources)
	}

	// --- Grouping into 5-minute windows ---
	// window 1: signals at 10:02 and 10:03 → bucket 10:00
	// window 2: signal at 10:12 → bucket 10:10

	if len(analysis.Groups) != 2 {
		t.Fatalf("Groups: want 2, got %d", len(analysis.Groups))
	}

	g1 := analysis.Groups[0]
	if !g1.WindowStart.Equal(time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("Groups[0].WindowStart: want 10:00, got %s", g1.WindowStart)
	}
	if !g1.WindowEnd.Equal(time.Date(2026, 5, 1, 10, 5, 0, 0, time.UTC)) {
		t.Errorf("Groups[0].WindowEnd: want 10:05, got %s", g1.WindowEnd)
	}
	if len(g1.Signals) != 2 {
		t.Errorf("Groups[0].Signals: want 2, got %d", len(g1.Signals))
	}
	if g1.SourceCount != 2 {
		t.Errorf("Groups[0].SourceCount: want 2, got %d", g1.SourceCount)
	}
	if g1.HighestSeverity != domain.SignalSeverityHigh {
		t.Errorf("Groups[0].HighestSeverity: want high, got %q", g1.HighestSeverity)
	}
	if g1.Service != "payments" {
		t.Errorf("Groups[0].Service: want %q, got %q", "payments", g1.Service)
	}
	if g1.Summary == "" {
		t.Error("Groups[0].Summary: must not be empty")
	}

	g2 := analysis.Groups[1]
	if !g2.WindowStart.Equal(time.Date(2026, 5, 1, 10, 10, 0, 0, time.UTC)) {
		t.Errorf("Groups[1].WindowStart: want 10:10, got %s", g2.WindowStart)
	}
	if len(g2.Signals) != 1 {
		t.Errorf("Groups[1].Signals: want 1, got %d", len(g2.Signals))
	}
	if g2.HighestSeverity != domain.SignalSeverityMedium {
		t.Errorf("Groups[1].HighestSeverity: want medium, got %q", g2.HighestSeverity)
	}

	// --- Summary and Confidence ---

	if analysis.Summary == "" {
		t.Error("Summary: must not be empty")
	}
	if !strings.Contains(analysis.Summary, "payments") {
		t.Errorf("Summary: expected to mention service name, got %q", analysis.Summary)
	}
	if analysis.Confidence <= 0 || analysis.Confidence > 1 {
		t.Errorf("Confidence: want (0, 1], got %v", analysis.Confidence)
	}

	// --- Deterministic sort: groups ordered by window start ---

	if !analysis.Groups[0].WindowStart.Before(analysis.Groups[1].WindowStart) {
		t.Error("Groups must be sorted by WindowStart ascending")
	}

	// --- Signals within a group sorted by timestamp then source ---

	sig0 := g1.Signals[0]
	sig1 := g1.Signals[1]
	if sig0.Timestamp.After(sig1.Timestamp) {
		t.Error("signals within group must be sorted by timestamp ascending")
	}
}

// TestIntegrationNoSignalsProducesEmptyAnalysis verifies that providers returning nothing
// produce a well-formed zero-confidence analysis with no groups.
func TestIntegrationNoSignalsProducesEmptyAnalysis(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	until := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)

	svc := NewService([]outbound.SignalProvider{
		signalProviderStub{name: "datadog", signals: nil},
		signalProviderStub{name: "elastic", signals: nil},
	}, noopLogger())
	svc.now = func() time.Time { return until }

	analysis, err := svc.Run(context.Background(), Input{
		Services: []string{"payments"},
		Since:   since,
		Until:   until,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Service != "payments" {
		t.Errorf("Service: want %q, got %q", "payments", analysis.Service)
	}
	if analysis.ModelVersion != domain.IncidentAnalysisModelVersion {
		t.Errorf("ModelVersion: want %q, got %q", domain.IncidentAnalysisModelVersion, analysis.ModelVersion)
	}
	if analysis.Metadata.TotalSignals != 0 {
		t.Errorf("TotalSignals: want 0, got %d", analysis.Metadata.TotalSignals)
	}
	if len(analysis.Groups) != 0 {
		t.Errorf("Groups: want 0, got %d", len(analysis.Groups))
	}
	if analysis.Confidence != 0 {
		t.Errorf("Confidence: want 0, got %v", analysis.Confidence)
	}
	if analysis.Summary == "" {
		t.Error("Summary: must not be empty even with no signals")
	}
	if analysis.TimeRange == "" {
		t.Error("TimeRange: must not be empty")
	}
}

// TestIntegrationSignalsOutsideWindowAreExcluded verifies that providers returning signals
// outside the requested time window do not appear in the analysis.
func TestIntegrationSignalsOutsideWindowAreExcluded(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	until := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)

	svc := NewService([]outbound.SignalProvider{
		signalProviderStub{
			name: "datadog",
			signals: []domain.Signal{
				{
					Source:    domain.SignalSourceDatadog,
					Service:   "payments",
					Severity:  domain.SignalSeverityHigh,
					Timestamp: since.Add(-time.Minute), // before window
				},
				{
					Source:    domain.SignalSourceDatadog,
					Service:   "payments",
					Severity:  domain.SignalSeverityHigh,
					Timestamp: until.Add(time.Minute), // after window
				},
			},
		},
	}, noopLogger())
	svc.now = func() time.Time { return until }

	analysis, err := svc.Run(context.Background(), Input{
		Services: []string{"payments"},
		Since:   since,
		Until:   until,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Metadata.TotalSignals != 0 {
		t.Errorf("TotalSignals: want 0 (all filtered), got %d", analysis.Metadata.TotalSignals)
	}
	if len(analysis.Groups) != 0 {
		t.Errorf("Groups: want 0, got %d", len(analysis.Groups))
	}
}
