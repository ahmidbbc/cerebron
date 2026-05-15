package analyzeincident

import (
	"context"
	"errors"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

var errStub = errors.New("stub error")

type deploymentProviderStub struct {
	name        string
	deployments []domain.Deployment
	err         error
}

func (d deploymentProviderStub) Name() string { return d.name }

func (d deploymentProviderStub) FetchDeployments(_ context.Context, _ outbound.DeploymentQuery) ([]domain.Deployment, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.deployments, nil
}

func TestCorrelateDeployments_NoProviders(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	svc := NewService(nil, noopLogger())
	input := Input{
		Services: []string{"api"},
		Since:    now.Add(-10 * time.Minute),
		Until:    now,
	}
	result, err := svc.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DeploymentContext != nil {
		t.Error("expected nil DeploymentContext when no deployment providers configured")
	}
}

func TestCorrelateDeployments_SuspectAndRollback(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	incidentStart := now.Add(-5 * time.Minute)

	// Deployment 30 minutes before the first signal — within suspectWindow (1h).
	suspectDeploy := domain.Deployment{
		ID:        "gh:1",
		Source:    "github",
		Service:   "api",
		Status:    domain.DeploymentStatusSuccess,
		StartedAt: incidentStart.Add(-30 * time.Minute),
	}
	// Old deployment 3 hours before — outside suspectWindow.
	oldDeploy := domain.Deployment{
		ID:        "gh:2",
		Source:    "github",
		Service:   "api",
		Status:    domain.DeploymentStatusSuccess,
		StartedAt: incidentStart.Add(-3 * time.Hour),
	}

	svc := NewService(
		[]outbound.SignalProvider{
			signalProviderStub{
				name: "datadog",
				signals: []domain.Signal{
					{
						Source:    domain.SignalSourceDatadog,
						Service:   "api",
						Type:      domain.SignalTypeMetric,
						Summary:   "error rate spike",
						Severity:  domain.SignalSeverityHigh,
						Timestamp: incidentStart,
					},
				},
			},
		},
		noopLogger(),
		WithDeploymentProviders([]outbound.DeploymentProvider{
			deploymentProviderStub{
				name:        "github",
				deployments: []domain.Deployment{suspectDeploy, oldDeploy},
			},
		}),
	)

	result, err := svc.Run(context.Background(), Input{
		Services: []string{"api"},
		Since:    now.Add(-10 * time.Minute),
		Until:    now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DeploymentContext == nil {
		t.Fatal("expected DeploymentContext to be set")
	}

	if len(result.DeploymentContext.RecentDeployments) != 2 {
		t.Errorf("expected 2 recent deployments, got %d", len(result.DeploymentContext.RecentDeployments))
	}
	if len(result.DeploymentContext.SuspectDeployments) != 1 {
		t.Errorf("expected 1 suspect deployment, got %d", len(result.DeploymentContext.SuspectDeployments))
	}
	if result.DeploymentContext.SuspectDeployments[0].ID != suspectDeploy.ID {
		t.Errorf("expected suspect deployment ID %q, got %q", suspectDeploy.ID, result.DeploymentContext.SuspectDeployments[0].ID)
	}
	if len(result.DeploymentContext.RollbackCandidates) != 1 {
		t.Errorf("expected 1 rollback candidate, got %d", len(result.DeploymentContext.RollbackCandidates))
	}
}

func TestCorrelateDeployments_NoGroups_NoSuspect(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	deploy := domain.Deployment{
		ID:        "gh:1",
		Source:    "github",
		Service:   "api",
		Status:    domain.DeploymentStatusSuccess,
		StartedAt: now.Add(-15 * time.Minute),
	}

	// No signal providers means no groups.
	svc := NewService(
		nil,
		noopLogger(),
		WithDeploymentProviders([]outbound.DeploymentProvider{
			deploymentProviderStub{name: "github", deployments: []domain.Deployment{deploy}},
		}),
	)

	result, err := svc.Run(context.Background(), Input{
		Services: []string{"api"},
		Since:    now.Add(-10 * time.Minute),
		Until:    now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DeploymentContext == nil {
		t.Fatal("expected DeploymentContext to be set")
	}
	if len(result.DeploymentContext.RecentDeployments) != 1 {
		t.Errorf("expected 1 recent deployment, got %d", len(result.DeploymentContext.RecentDeployments))
	}
	if len(result.DeploymentContext.SuspectDeployments) != 0 {
		t.Errorf("expected 0 suspect deployments when no signal groups, got %d", len(result.DeploymentContext.SuspectDeployments))
	}
	if len(result.DeploymentContext.RollbackCandidates) != 0 {
		t.Errorf("expected 0 rollback candidates when no signal groups, got %d", len(result.DeploymentContext.RollbackCandidates))
	}
}

func TestCorrelateDeployments_FailingProviderIsSkipped(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	svc := NewService(
		[]outbound.SignalProvider{
			signalProviderStub{name: "datadog", signals: nil},
		},
		noopLogger(),
		WithDeploymentProviders([]outbound.DeploymentProvider{
			deploymentProviderStub{name: "github", err: errStub},
		}),
	)

	result, err := svc.Run(context.Background(), Input{
		Services: []string{"api"},
		Since:    now.Add(-10 * time.Minute),
		Until:    now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DeploymentContext == nil {
		t.Fatal("expected DeploymentContext even when provider fails")
	}
	if len(result.DeploymentContext.RecentDeployments) != 0 {
		t.Errorf("expected 0 recent deployments, got %d", len(result.DeploymentContext.RecentDeployments))
	}
}
