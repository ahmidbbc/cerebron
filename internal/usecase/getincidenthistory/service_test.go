package getincidenthistory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/getincidenthistory"
)

type stubRepo struct {
	byService map[string][]domain.StoredIncident
	err       error
}

func (r stubRepo) Save(_ context.Context, _ domain.StoredIncident) (domain.StoredIncident, error) {
	return domain.StoredIncident{}, nil
}
func (r stubRepo) FindByFingerprint(_ context.Context, _ string) (domain.StoredIncident, bool, error) {
	return domain.StoredIncident{}, false, nil
}
func (r stubRepo) ListByService(_ context.Context, service string) ([]domain.StoredIncident, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.byService[service], nil
}
func (r stubRepo) ListAll(_ context.Context) ([]domain.StoredIncident, error) {
	return nil, nil
}

func TestGetIncidentHistory_RequiresService(t *testing.T) {
	t.Parallel()
	svc := getincidenthistory.NewService(stubRepo{})
	_, err := svc.Run(context.Background(), getincidenthistory.Input{})
	if err == nil {
		t.Fatal("expected error for empty service")
	}
}

func TestGetIncidentHistory_ReturnsIncidents(t *testing.T) {
	t.Parallel()
	repo := stubRepo{
		byService: map[string][]domain.StoredIncident{
			"api": {
				{ID: "i1", Service: "api", CreatedAt: time.Now()},
				{ID: "i2", Service: "api", CreatedAt: time.Now().Add(-1 * time.Hour)},
			},
		},
	}
	svc := getincidenthistory.NewService(repo)
	result, err := svc.Run(context.Background(), getincidenthistory.Input{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Incidents) != 2 {
		t.Fatalf("expected 2 incidents, got %d", len(result.Incidents))
	}
	if result.Total != 2 {
		t.Fatalf("expected total=2, got %d", result.Total)
	}
}

func TestGetIncidentHistory_RepoError(t *testing.T) {
	t.Parallel()
	repo := stubRepo{err: fmt.Errorf("db down")}
	svc := getincidenthistory.NewService(repo)
	_, err := svc.Run(context.Background(), getincidenthistory.Input{Service: "api"})
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}

func TestGetIncidentHistory_RespectsLimit(t *testing.T) {
	t.Parallel()
	incidents := make([]domain.StoredIncident, 10)
	for i := range incidents {
		incidents[i] = domain.StoredIncident{ID: fmt.Sprintf("i%d", i), Service: "api"}
	}
	repo := stubRepo{byService: map[string][]domain.StoredIncident{"api": incidents}}
	svc := getincidenthistory.NewService(repo)
	result, err := svc.Run(context.Background(), getincidenthistory.Input{Service: "api", Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Incidents) != 3 {
		t.Fatalf("expected 3 incidents, got %d", len(result.Incidents))
	}
	if result.Total != 10 {
		t.Fatalf("expected total=10, got %d", result.Total)
	}
}

func TestGetIncidentHistory_EmptyResultIsNotNil(t *testing.T) {
	t.Parallel()
	repo := stubRepo{byService: map[string][]domain.StoredIncident{}}
	svc := getincidenthistory.NewService(repo)
	result, err := svc.Run(context.Background(), getincidenthistory.Input{Service: "unknown"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Incidents == nil {
		t.Fatal("expected non-nil incidents slice")
	}
}
