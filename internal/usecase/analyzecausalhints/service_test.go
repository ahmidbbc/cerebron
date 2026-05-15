package analyzecausalhints_test

import (
	"context"
	"testing"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/analyzecausalhints"
)

var svc = analyzecausalhints.NewService()

func TestNoHintsForEmptyAnalysis(t *testing.T) {
	t.Parallel()

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{
		Analysis: domain.IncidentAnalysis{Service: "api"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Hints) != 0 {
		t.Errorf("expected no hints, got %d", len(result.Hints))
	}
	if result.Service != "api" {
		t.Errorf("expected service api, got %s", result.Service)
	}
}

func TestDeploymentTriggeredHintFires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	analysis := domain.IncidentAnalysis{
		Service: "payments",
		Groups: []domain.SignalGroup{
			{Service: "payments", WindowStart: now, WindowEnd: now.Add(5 * time.Minute)},
		},
		DeploymentContext: &domain.DeploymentContext{
			SuspectDeployments: []domain.Deployment{
				{ID: "deploy-1", Service: "payments", StartedAt: now.Add(-30 * time.Minute)},
			},
		},
	}

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(result.Hints, domain.CausalRuleDeploymentTriggered) {
		t.Errorf("expected deployment_triggered hint, got %v", result.Hints)
	}
}

func TestDeploymentTriggeredDoesNotFireWhenDeployAfterGroup(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	analysis := domain.IncidentAnalysis{
		Service: "payments",
		Groups: []domain.SignalGroup{
			{Service: "payments", WindowStart: now, WindowEnd: now.Add(5 * time.Minute)},
		},
		DeploymentContext: &domain.DeploymentContext{
			SuspectDeployments: []domain.Deployment{
				{ID: "deploy-2", Service: "payments", StartedAt: now.Add(10 * time.Minute)},
			},
		},
	}

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasRule(result.Hints, domain.CausalRuleDeploymentTriggered) {
		t.Errorf("expected no deployment_triggered hint when deploy after group")
	}
}

func TestDatabaseLatencyHintFires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	analysis := domain.IncidentAnalysis{
		Service: "api",
		Groups: []domain.SignalGroup{
			{
				Service:     "postgres",
				WindowStart: now,
				WindowEnd:   now.Add(5 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeMetric, Service: "postgres"}},
			},
			{
				Service:     "api",
				WindowStart: now.Add(2 * time.Minute),
				WindowEnd:   now.Add(7 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeLog, Service: "api"}},
			},
		},
	}

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(result.Hints, domain.CausalRuleDatabaseLatency) {
		t.Errorf("expected database_latency hint, got %v", result.Hints)
	}
}

func TestDatabaseLatencyHintDoesNotFireWhenAPIFirst(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	analysis := domain.IncidentAnalysis{
		Service: "api",
		Groups: []domain.SignalGroup{
			{
				Service:     "api",
				WindowStart: now,
				WindowEnd:   now.Add(5 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeLog, Service: "api"}},
			},
			{
				Service:     "postgres",
				WindowStart: now.Add(3 * time.Minute),
				WindowEnd:   now.Add(8 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeMetric, Service: "postgres"}},
			},
		},
	}

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasRule(result.Hints, domain.CausalRuleDatabaseLatency) {
		t.Errorf("expected no database_latency hint when API fails first")
	}
}

func TestInfraDegradationHintFires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	analysis := domain.IncidentAnalysis{
		Service: "checkout",
		Groups: []domain.SignalGroup{
			{
				Service:     "infra-node",
				WindowStart: now,
				WindowEnd:   now.Add(5 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeMetric, Service: "infra-node"}},
			},
			{
				Service:     "checkout",
				WindowStart: now.Add(3 * time.Minute),
				WindowEnd:   now.Add(8 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeLog, Service: "checkout"}},
			},
		},
	}

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(result.Hints, domain.CausalRuleInfraDegradation) {
		t.Errorf("expected infra_degradation hint, got %v", result.Hints)
	}
}

func TestInfraDegradationHintDoesNotFireForSameService(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	analysis := domain.IncidentAnalysis{
		Service: "api",
		Groups: []domain.SignalGroup{
			{
				Service:     "api",
				WindowStart: now,
				WindowEnd:   now.Add(5 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeMetric, Service: "api"}},
			},
			{
				Service:     "api",
				WindowStart: now.Add(3 * time.Minute),
				WindowEnd:   now.Add(8 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeLog, Service: "api"}},
			},
		},
	}

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasRule(result.Hints, domain.CausalRuleInfraDegradation) {
		t.Errorf("expected no infra_degradation hint for same service")
	}
}

func TestDatabaseLatencyDoesNotFireOnPureInfraInput(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	analysis := domain.IncidentAnalysis{
		Service: "checkout",
		Groups: []domain.SignalGroup{
			{
				Service:     "infra-node",
				WindowStart: now,
				WindowEnd:   now.Add(5 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeMetric, Service: "infra-node"}},
			},
			{
				Service:     "checkout",
				WindowStart: now.Add(3 * time.Minute),
				WindowEnd:   now.Add(8 * time.Minute),
				Signals:     []domain.Signal{{Type: domain.SignalTypeLog, Service: "checkout"}},
			},
		},
	}

	result, err := svc.Run(context.Background(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasRule(result.Hints, domain.CausalRuleDatabaseLatency) {
		t.Errorf("database_latency must not fire when metric group is not a DB-named service")
	}
	if !hasRule(result.Hints, domain.CausalRuleInfraDegradation) {
		t.Errorf("infra_degradation must fire for this input")
	}
}

func hasRule(hints []domain.CausalHint, rule domain.CausalRule) bool {
	for _, h := range hints {
		if h.Rule == rule {
			return true
		}
	}
	return false
}
