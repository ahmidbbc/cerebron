package detectincidenttrends

import (
	"context"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/storage"
)

func makeIncident(service, fingerprint string, severity domain.SignalSeverity, createdAt time.Time) domain.StoredIncident {
	return domain.StoredIncident{
		Fingerprint: fingerprint,
		Service:     service,
		Analysis: domain.IncidentAnalysis{
			Service: service,
			Groups: []domain.SignalGroup{
				{HighestSeverity: severity},
			},
		},
		CreatedAt:       createdAt,
		RecurrenceCount: 1,
	}
}

func seedRepo(t *testing.T, incidents ...domain.StoredIncident) *storage.MemoryIncidentRepository {
	t.Helper()
	repo := storage.NewMemoryIncidentRepository()
	for _, inc := range incidents {
		if _, err := repo.Save(context.Background(), inc); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return repo
}

func TestRunEmptyRepoReturnsEmptyTrends(t *testing.T) {
	t.Parallel()

	svc := NewService(storage.NewMemoryIncidentRepository())
	trends, err := svc.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trends.Services) != 0 {
		t.Fatalf("expected 0 services, got %d", len(trends.Services))
	}
}

func TestRunReturnsOneServiceTrend(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repo := seedRepo(t,
		makeIncident("api", "fp-1", domain.SignalSeverityHigh, now.Add(-2*24*time.Hour)),
		makeIncident("api", "fp-2", domain.SignalSeverityMedium, now.Add(-24*time.Hour)),
		makeIncident("api", "fp-3", domain.SignalSeverityLow, now),
	)
	svc := NewService(repo)

	trends, err := svc.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trends.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(trends.Services))
	}
	st := trends.Services[0]
	if st.Service != "api" {
		t.Fatalf("expected service api, got %q", st.Service)
	}
	if st.IncidentCount != 3 {
		t.Fatalf("expected 3 incidents, got %d", st.IncidentCount)
	}
	if st.RecurrenceTotal != 3 {
		t.Fatalf("expected recurrence total 3, got %d", st.RecurrenceTotal)
	}
}

func TestRunFiltersByService(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repo := seedRepo(t,
		makeIncident("api", "fp-1", domain.SignalSeverityHigh, now),
		makeIncident("worker", "fp-2", domain.SignalSeverityLow, now),
	)
	svc := NewService(repo)

	trends, err := svc.Run(context.Background(), Input{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trends.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(trends.Services))
	}
	if trends.Services[0].Service != "api" {
		t.Fatalf("expected api, got %q", trends.Services[0].Service)
	}
}

func TestRunCountsDegradingAndStable(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	// "degrading": older incidents low severity, newer high severity
	repo := seedRepo(t,
		makeIncident("bad-svc", "fp-1", domain.SignalSeverityLow, now.Add(-10*24*time.Hour)),
		makeIncident("bad-svc", "fp-2", domain.SignalSeverityHigh, now),
		makeIncident("stable-svc", "fp-3", domain.SignalSeverityMedium, now.Add(-5*24*time.Hour)),
		makeIncident("stable-svc", "fp-4", domain.SignalSeverityMedium, now),
	)
	svc := NewService(repo)

	trends, err := svc.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trends.DegradingCount+trends.StableCount+trends.ImprovingCount != len(trends.Services) {
		t.Fatal("trend counts must sum to number of services")
	}
	if trends.DegradingCount < 1 {
		t.Fatalf("expected at least 1 degrading service (bad-svc), got %d", trends.DegradingCount)
	}
}

func TestRunMultipleServicesOrderedByFrequency(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	// "busy" has 3 incidents in 1 day; "quiet" has 1 incident in 10 days
	repo := seedRepo(t,
		makeIncident("busy", "fp-1", domain.SignalSeverityMedium, now.Add(-23*time.Hour)),
		makeIncident("busy", "fp-2", domain.SignalSeverityMedium, now.Add(-12*time.Hour)),
		makeIncident("busy", "fp-3", domain.SignalSeverityMedium, now),
		makeIncident("quiet", "fp-4", domain.SignalSeverityLow, now.Add(-10*24*time.Hour)),
	)
	svc := NewService(repo)

	trends, err := svc.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trends.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(trends.Services))
	}
	if trends.Services[0].Service != "busy" {
		t.Fatalf("expected busy to be first (highest frequency), got %q", trends.Services[0].Service)
	}
}
