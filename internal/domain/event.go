package domain

import "time"

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityAlert    Severity = "alert"
	SeverityCritical Severity = "critical"
)

type EventSource string

const (
	SourceDatadog       EventSource = "datadog"
	SourceElasticsearch EventSource = "elasticsearch"
)

type Event struct {
	ID          string
	Source      EventSource
	Service     string
	Environment string
	Severity    Severity
	StatusCode  int
	StatusClass string
	Route       string
	Message     string
	Error       string
	OccurredAt  time.Time
	Fingerprint string
	Summary     string
	OwnerTeam   string
	Attributes  map[string]string
}

type LogQuery struct {
	Service     string
	Environment string
	Since       time.Time
	Until       time.Time
	Terms       []string
}
