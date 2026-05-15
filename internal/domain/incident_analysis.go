package domain

const IncidentAnalysisModelVersion = "v1"

type IncidentAnalysis struct {
	Service           string             `json:"service"`
	TimeRange         string             `json:"time_range"`
	ModelVersion      string             `json:"model_version"`
	Metadata          Metadata           `json:"metadata"`
	Groups            []SignalGroup       `json:"groups"`
	Summary           string             `json:"summary"`
	Confidence        float64            `json:"confidence"`
	DeploymentContext *DeploymentContext `json:"deployment_context,omitempty"`
}

// DeploymentContext attaches deployment correlation data to an IncidentAnalysis.
type DeploymentContext struct {
	RecentDeployments  []Deployment `json:"recent_deployments"`
	SuspectDeployments []Deployment `json:"suspect_deployments"`
	RollbackCandidates []Deployment `json:"rollback_candidates"`
}
