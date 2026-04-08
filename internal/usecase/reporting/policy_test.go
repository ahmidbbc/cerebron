package reporting

import (
	"testing"
	"time"

	"cerebron/internal/domain"
)

func TestPolicyShouldReportCriticalIncident(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	events := []domain.Event{
		{
			Severity:    domain.SeverityCritical,
			Service:     "catalog-api",
			Environment: "preprod",
			OccurredAt:  time.Now(),
		},
	}

	if !policy.ShouldReport(events) {
		t.Fatalf("expected critical event to be reportable")
	}
}

func TestPolicyShouldNotReportLowSignalInfoEvent(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	events := []domain.Event{
		{
			Severity:   domain.SeverityInfo,
			OccurredAt: time.Now(),
		},
	}

	if policy.ShouldReport(events) {
		t.Fatalf("expected low-signal info event to be ignored")
	}
}

func TestPolicyScoreIncludesCorrelationSignal(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	score := policy.Score([]domain.Event{{
		Severity:      domain.SeverityWarning,
		Service:       "import-ads-booking-private-api",
		Environment:   "prod-morpheus",
		OccurredAt:    time.Now(),
		CorrelatedIDs: []string{"dd-1"},
	}})

	if score != 65 {
		t.Fatalf("expected correlation signal to increase score to 65, got %d", score)
	}
}
