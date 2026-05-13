package storage

import (
	"context"
	"testing"
	"time"

	"cerebron/internal/domain"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func makeStoredIncident(fp, service string) domain.StoredIncident {
	return domain.StoredIncident{
		Fingerprint: fp,
		Service:     service,
		Analysis:    domain.IncidentAnalysis{Service: service, ModelVersion: "v1"},
	}
}

func repoWithFixedTime(t time.Time) *MemoryIncidentRepository {
	r := NewMemoryIncidentRepository()
	r.now = func() time.Time { return t }
	return r
}

func TestMemoryRepo_SaveAndFindByFingerprint(t *testing.T) {
	ctx := context.Background()
	repo := repoWithFixedTime(epoch)

	inc := makeStoredIncident("abc123", "svc-a")
	saved, err := repo.Save(ctx, inc)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if saved.RecurrenceCount != 1 {
		t.Fatalf("expected recurrence_count=1, got %d", saved.RecurrenceCount)
	}

	found, ok, err := repo.FindByFingerprint(ctx, "abc123")
	if err != nil {
		t.Fatalf("FindByFingerprint: %v", err)
	}
	if !ok {
		t.Fatal("expected to find incident by fingerprint")
	}
	if found.ID != saved.ID {
		t.Fatalf("found ID %s != saved ID %s", found.ID, saved.ID)
	}
}

func TestMemoryRepo_RecurrenceIncrement(t *testing.T) {
	ctx := context.Background()
	repo := repoWithFixedTime(epoch)

	inc := makeStoredIncident("fp-dup", "svc-b")
	repo.Save(ctx, inc)
	second, err := repo.Save(ctx, inc)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if second.RecurrenceCount != 2 {
		t.Fatalf("expected recurrence_count=2, got %d", second.RecurrenceCount)
	}
}

func TestMemoryRepo_FindByFingerprint_NotFound(t *testing.T) {
	repo := repoWithFixedTime(epoch)
	_, ok, err := repo.FindByFingerprint(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected not found")
	}
}

func TestMemoryRepo_ListByService(t *testing.T) {
	ctx := context.Background()

	t1 := epoch
	t2 := epoch.Add(time.Second)

	calls := []time.Time{t1, t2, t1}
	idx := 0
	repo := NewMemoryIncidentRepository()
	repo.now = func() time.Time {
		v := calls[idx]
		idx++
		return v
	}

	saved1, _ := repo.Save(ctx, makeStoredIncident("fp1", "svc-a"))
	saved2, _ := repo.Save(ctx, makeStoredIncident("fp2", "svc-a"))
	repo.Save(ctx, makeStoredIncident("fp3", "svc-b"))

	list, err := repo.ListByService(ctx, "svc-a")
	if err != nil {
		t.Fatalf("ListByService: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 incidents for svc-a, got %d", len(list))
	}
	// newest first: saved2 (t2) before saved1 (t1)
	if list[0].ID != saved2.ID || list[1].ID != saved1.ID {
		t.Fatalf("expected order [%s, %s], got [%s, %s]", saved2.ID, saved1.ID, list[0].ID, list[1].ID)
	}
}
