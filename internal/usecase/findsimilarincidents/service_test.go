package findsimilarincidents

import (
	"context"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/storage"
)

func makeStored(fingerprint, service string) domain.StoredIncident {
	return domain.StoredIncident{
		Fingerprint:     fingerprint,
		Service:         service,
		Analysis:        domain.IncidentAnalysis{Service: service},
		CreatedAt:       time.Now().UTC(),
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

func TestRunReturnsExactMatchByFingerprint(t *testing.T) {
	t.Parallel()

	repo := seedRepo(t, makeStored("fp-abc", "catalog-api"))
	svc := NewService(repo)

	result, err := svc.Run(context.Background(), Input{Fingerprint: "fp-abc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExactMatch == nil {
		t.Fatal("expected exact match, got nil")
	}
	if result.ExactMatch.Fingerprint != "fp-abc" {
		t.Fatalf("expected fingerprint fp-abc, got %q", result.ExactMatch.Fingerprint)
	}
	if len(result.Related) != 0 {
		t.Fatalf("expected no related (no service filter), got %d", len(result.Related))
	}
}

func TestRunReturnsRelatedByService(t *testing.T) {
	t.Parallel()

	repo := seedRepo(t,
		makeStored("fp-1", "catalog-api"),
		makeStored("fp-2", "catalog-api"),
		makeStored("fp-3", "billing-api"),
	)
	svc := NewService(repo)

	result, err := svc.Run(context.Background(), Input{Service: "catalog-api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Related) != 2 {
		t.Fatalf("expected 2 related, got %d", len(result.Related))
	}
}

func TestRunExcludesExactMatchFromRelated(t *testing.T) {
	t.Parallel()

	repo := seedRepo(t,
		makeStored("fp-1", "catalog-api"),
		makeStored("fp-2", "catalog-api"),
	)
	svc := NewService(repo)

	result, err := svc.Run(context.Background(), Input{
		Fingerprint: "fp-1",
		Service:     "catalog-api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExactMatch == nil {
		t.Fatal("expected exact match")
	}
	for _, rel := range result.Related {
		if rel.ID == result.ExactMatch.ID {
			t.Fatal("exact match must not appear in related list")
		}
	}
}

func TestRunRespectsLimit(t *testing.T) {
	t.Parallel()

	repo := seedRepo(t,
		makeStored("fp-1", "svc"),
		makeStored("fp-2", "svc"),
		makeStored("fp-3", "svc"),
	)
	svc := NewService(repo)

	result, err := svc.Run(context.Background(), Input{Service: "svc", Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Related) != 2 {
		t.Fatalf("expected exactly 2 related, got %d", len(result.Related))
	}
}

func TestRunReturnsErrorWithoutFingerprintOrService(t *testing.T) {
	t.Parallel()

	svc := NewService(storage.NewMemoryIncidentRepository())
	_, err := svc.Run(context.Background(), Input{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestRunReturnsEmptyRelatedSlice(t *testing.T) {
	t.Parallel()

	svc := NewService(storage.NewMemoryIncidentRepository())
	result, err := svc.Run(context.Background(), Input{Service: "unknown"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Related == nil {
		t.Fatal("expected non-nil related slice")
	}
}
