package analyzecausalhints

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cerebron/internal/domain"
)

// Input to the causal hint analysis.
type Input struct {
	Analysis domain.IncidentAnalysis
}

type Service struct{}

func NewService() Service { return Service{} }

// Run applies deterministic causal heuristics to the incident analysis and returns
// a CausalAnalysis with all hints that fired.
func (s Service) Run(_ context.Context, input Input) (domain.CausalAnalysis, error) {
	a := input.Analysis
	hints := []domain.CausalHint{}

	if h, ok := deploymentTriggered(a); ok {
		hints = append(hints, h)
	}
	if h, ok := databaseLatencyBeforeAPIFailure(a); ok {
		hints = append(hints, h)
	}
	if h, ok := infraDegradationBeforeServiceInstability(a); ok {
		hints = append(hints, h)
	}

	return domain.CausalAnalysis{
		Service: a.Service,
		Hints:   hints,
	}, nil
}

// deploymentTriggered fires when a suspect deployment precedes the first signal group window.
func deploymentTriggered(a domain.IncidentAnalysis) (domain.CausalHint, bool) {
	if a.DeploymentContext == nil || len(a.DeploymentContext.SuspectDeployments) == 0 {
		return domain.CausalHint{}, false
	}
	firstGroup := earliestGroup(a.Groups)
	if firstGroup == nil {
		return domain.CausalHint{}, false
	}

	for _, d := range a.DeploymentContext.SuspectDeployments {
		if !d.StartedAt.IsZero() && d.StartedAt.Before(firstGroup.WindowStart) {
			return domain.CausalHint{
				Rule:       domain.CausalRuleDeploymentTriggered,
				Confidence: 0.8,
				Evidence:   fmt.Sprintf("deployment %s (%s) started %s before first incident group", d.ID, d.Service, firstGroup.WindowStart.Sub(d.StartedAt).Truncate(time.Second)),
			}, true
		}
	}
	return domain.CausalHint{}, false
}

// databaseLatencyBeforeAPIFailure fires when a metric signal from a database-like service
// appears earlier than a log signal from an API-like service.
func databaseLatencyBeforeAPIFailure(a domain.IncidentAnalysis) (domain.CausalHint, bool) {
	var dbGroup, apiGroup *domain.SignalGroup

	for i := range a.Groups {
		g := &a.Groups[i]
		if isDBGroup(g) && dbGroup == nil {
			dbGroup = g
		}
		if isAPIGroup(g) && apiGroup == nil {
			apiGroup = g
		}
	}

	if dbGroup == nil || apiGroup == nil {
		return domain.CausalHint{}, false
	}
	if !dbGroup.WindowStart.Before(apiGroup.WindowStart) {
		return domain.CausalHint{}, false
	}

	lag := apiGroup.WindowStart.Sub(dbGroup.WindowStart).Truncate(time.Second)
	return domain.CausalHint{
		Rule:       domain.CausalRuleDatabaseLatency,
		Confidence: 0.75,
		Evidence:   fmt.Sprintf("database signals in group %q appeared %s before API signals in group %q", dbGroup.Service, lag, apiGroup.Service),
	}, true
}

// infraDegradationBeforeServiceInstability fires when a metric signal precedes a log signal
// across different services, suggesting infra triggered service failures.
func infraDegradationBeforeServiceInstability(a domain.IncidentAnalysis) (domain.CausalHint, bool) {
	var metricGroup, logGroup *domain.SignalGroup

	for i := range a.Groups {
		g := &a.Groups[i]
		if hasSignalType(g, domain.SignalTypeMetric) && metricGroup == nil {
			metricGroup = g
		}
		if hasSignalType(g, domain.SignalTypeLog) && logGroup == nil {
			logGroup = g
		}
	}

	if metricGroup == nil || logGroup == nil {
		return domain.CausalHint{}, false
	}
	// Only fire if the metric group is from a different service.
	if metricGroup.Service == logGroup.Service {
		return domain.CausalHint{}, false
	}
	if !metricGroup.WindowStart.Before(logGroup.WindowStart) {
		return domain.CausalHint{}, false
	}

	lag := logGroup.WindowStart.Sub(metricGroup.WindowStart).Truncate(time.Second)
	return domain.CausalHint{
		Rule:       domain.CausalRuleInfraDegradation,
		Confidence: 0.7,
		Evidence:   fmt.Sprintf("infra metric degradation in %q appeared %s before service instability in %q", metricGroup.Service, lag, logGroup.Service),
	}, true
}

func earliestGroup(groups []domain.SignalGroup) *domain.SignalGroup {
	if len(groups) == 0 {
		return nil
	}
	earliest := &groups[0]
	for i := 1; i < len(groups); i++ {
		if groups[i].WindowStart.Before(earliest.WindowStart) {
			earliest = &groups[i]
		}
	}
	return earliest
}

func isDBGroup(g *domain.SignalGroup) bool {
	svc := strings.ToLower(g.Service)
	return strings.Contains(svc, "db") ||
		strings.Contains(svc, "database") ||
		strings.Contains(svc, "postgres") ||
		strings.Contains(svc, "mysql") ||
		strings.Contains(svc, "redis") ||
		strings.Contains(svc, "mongo")
}

func isAPIGroup(g *domain.SignalGroup) bool {
	svc := strings.ToLower(g.Service)
	return strings.Contains(svc, "api") ||
		strings.Contains(svc, "service") ||
		strings.Contains(svc, "server")
}

func hasSignalType(g *domain.SignalGroup, t string) bool {
	for _, s := range g.Signals {
		if s.Type == t {
			return true
		}
	}
	return false
}
