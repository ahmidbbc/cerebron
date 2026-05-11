package domain

type Metadata struct {
	TotalSignals    int `json:"total_signals"`
	DistinctSources int `json:"distinct_sources"`
}
