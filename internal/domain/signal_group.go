package domain

import "time"

type SignalGroup struct {
	Service         string    `json:"service"`
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	SourceCount     int       `json:"source_count"`
	HighestSeverity SignalSeverity `json:"highest_severity"`
	Summary         string    `json:"summary"`
	Signals         []Signal  `json:"signals"`
}
