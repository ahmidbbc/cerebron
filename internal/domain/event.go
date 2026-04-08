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
	SourceGerrit        EventSource = "gerrit"
	SourceGitHub        EventSource = "github"
)

type Event struct {
	ID            string
	Source        EventSource
	Service       string
	Environment   string
	Severity      Severity
	StatusCode    int
	StatusClass   string
	Route         string
	Message       string
	Error         string
	OccurredAt    time.Time
	Fingerprint   string
	Summary       string
	OwnerTeam     string
	Attributes    map[string]string
	ChangeRefs    []string
	CorrelatedIDs []string
}

type LogQuery struct {
	Service     string
	Environment string
	Since       time.Time
	Until       time.Time
	Terms       []string
}

type CodeLookupQuery struct {
	Service     string
	Environment string
	Since       time.Time
	ChangeRefs  []string
	Paths       []string
}

type CodeChange struct {
	ID         string
	Provider   EventSource
	Repository string
	Title      string
	Author     string
	ReviewedBy []string
	OwnerTeam  string
	MergedAt   time.Time
	Files      []string
	References []string
}

type ReportMessage struct {
	IncidentID  string
	ChannelID   string
	Title       string
	Summary     string
	Severity    Severity
	Service     string
	Environment string
	OwnerTeam   string
	Confidence  float64
	References  []string
}

type ReasoningInput struct {
	Events      []Event
	CodeChanges []CodeChange
}

type ReasoningOutput struct {
	Hypothesis         string
	ProposedResolution string
	OwnerTeam          string
	Confidence         float64
}
