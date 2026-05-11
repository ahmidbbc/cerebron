package domain

import "time"

const (
	SignalSourceDatadog = "datadog"
	SignalSourceElastic = "elastic"
	SignalSourceAWS     = "aws"
)

const (
	SignalTypeMetric = "metric"
	SignalTypeLog    = "log"
)

type Signal struct {
	Source    string            `json:"source"`
	Service   string            `json:"service"`
	Type      string            `json:"type"`
	Summary   string            `json:"summary"`
	Severity  SignalSeverity    `json:"severity"`
	Timestamp time.Time         `json:"timestamp"`
	Count     int               `json:"count,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}
