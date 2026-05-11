package domain

const IncidentAnalysisModelVersion = "v1"

type IncidentAnalysis struct {
	Service      string        `json:"service"`
	TimeRange    string        `json:"time_range"`
	ModelVersion string        `json:"model_version"`
	Metadata     Metadata      `json:"metadata"`
	Groups       []SignalGroup `json:"groups"`
	Summary      string        `json:"summary"`
	Confidence   float64       `json:"confidence"`
}
