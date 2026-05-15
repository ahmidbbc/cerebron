package getrecentdeployments_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
	"cerebron/internal/usecase/getrecentdeployments"
)

type stubProvider struct {
	name        string
	deployments []domain.Deployment
	err         error
}

func (s stubProvider) Name() string { return s.name }
func (s stubProvider) FetchDeployments(_ context.Context, _ outbound.DeploymentQuery) ([]domain.Deployment, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.deployments, nil
}

func TestGetRecentDeployments_RequiresService(t *testing.T) {
	t.Parallel()
	svc := getrecentdeployments.NewService(nil)
	_, err := svc.Run(context.Background(), getrecentdeployments.Input{})
	if err == nil {
		t.Fatal("expected error for empty service")
	}
}

func TestGetRecentDeployments_ReturnsDeployments(t *testing.T) {
	t.Parallel()
	now := time.Now()
	p := stubProvider{
		name: "github",
		deployments: []domain.Deployment{
			{ID: "d1", Service: "api", StartedAt: now.Add(-1 * time.Hour)},
			{ID: "d2", Service: "api", StartedAt: now.Add(-2 * time.Hour)},
		},
	}
	svc := getrecentdeployments.NewService([]outbound.DeploymentProvider{p})
	result, err := svc.Run(context.Background(), getrecentdeployments.Input{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Deployments) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(result.Deployments))
	}
	// newest first
	if result.Deployments[0].ID != "d1" {
		t.Errorf("expected d1 first, got %s", result.Deployments[0].ID)
	}
}

func TestGetRecentDeployments_SkipsFailingProvider(t *testing.T) {
	t.Parallel()
	p1 := stubProvider{name: "bad", err: errors.New("unavailable")}
	p2 := stubProvider{
		name:        "good",
		deployments: []domain.Deployment{{ID: "d1", Service: "api"}},
	}
	svc := getrecentdeployments.NewService([]outbound.DeploymentProvider{p1, p2})
	result, err := svc.Run(context.Background(), getrecentdeployments.Input{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Deployments) != 1 {
		t.Fatalf("expected 1 deployment from good provider, got %d", len(result.Deployments))
	}
}

func TestGetRecentDeployments_RespectsLimit(t *testing.T) {
	t.Parallel()
	deployments := make([]domain.Deployment, 10)
	for i := range deployments {
		deployments[i] = domain.Deployment{ID: "d", Service: "api"}
	}
	p := stubProvider{name: "src", deployments: deployments}
	svc := getrecentdeployments.NewService([]outbound.DeploymentProvider{p})
	result, err := svc.Run(context.Background(), getrecentdeployments.Input{Service: "api", Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Deployments) != 3 {
		t.Fatalf("expected 3 deployments, got %d", len(result.Deployments))
	}
}

func TestGetRecentDeployments_EmptyResultIsNotNil(t *testing.T) {
	t.Parallel()
	svc := getrecentdeployments.NewService(nil)
	result, err := svc.Run(context.Background(), getrecentdeployments.Input{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Deployments == nil {
		t.Fatal("expected non-nil deployments slice")
	}
}
