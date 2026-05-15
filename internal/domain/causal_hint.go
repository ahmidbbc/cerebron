package domain

// CausalRule identifies which deterministic heuristic fired.
type CausalRule string

const (
	// CausalRuleDeploymentTriggered fires when a suspect deployment precedes the first signal group.
	CausalRuleDeploymentTriggered CausalRule = "deployment_triggered"

	// CausalRuleDatabaseLatency fires when database-related signals precede API failure signals.
	CausalRuleDatabaseLatency CausalRule = "database_latency_before_api_failure"

	// CausalRuleInfraDegradation fires when infrastructure metric signals precede service log failures.
	CausalRuleInfraDegradation CausalRule = "infra_degradation_before_service_instability"
)

// CausalHint is a single deterministic causal observation for an incident.
type CausalHint struct {
	Rule       CausalRule `json:"rule"`
	Confidence float64    `json:"confidence"`
	Evidence   string     `json:"evidence"`
}

// CausalAnalysis aggregates all causal hints for an incident.
type CausalAnalysis struct {
	Service string       `json:"service"`
	Hints   []CausalHint `json:"hints"`
}
