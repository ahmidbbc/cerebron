package domain

import "time"

// TrendDirection describes whether a metric is worsening, stable, or improving.
type TrendDirection string

const (
	TrendDirectionWorsening TrendDirection = "worsening"
	TrendDirectionStable    TrendDirection = "stable"
	TrendDirectionImproving TrendDirection = "improving"
)

// ServiceTrend aggregates trend signals for a single service.
type ServiceTrend struct {
	Service          string         `json:"service"`
	IncidentCount    int            `json:"incident_count"`
	RecurrenceTotal  int            `json:"recurrence_total"`
	FrequencyPerDay  float64        `json:"frequency_per_day"`
	DominantSeverity SignalSeverity `json:"dominant_severity"`
	SeverityTrend    TrendDirection `json:"severity_trend"`
	FirstSeen        time.Time      `json:"first_seen"`
	LastSeen         time.Time      `json:"last_seen"`
}

// IncidentTrends is the output of trend detection across all stored incidents.
type IncidentTrends struct {
	Services        []ServiceTrend `json:"services"`
	DegradingCount  int            `json:"degrading_count"`
	StableCount     int            `json:"stable_count"`
	ImprovingCount  int            `json:"improving_count"`
	ObservationDays float64        `json:"observation_days"`
}
