package domain

// SignalSeverity is the normalized severity level for signals.
type SignalSeverity string

const (
	SignalSeverityLow    SignalSeverity = "low"
	SignalSeverityMedium SignalSeverity = "medium"
	SignalSeverityHigh   SignalSeverity = "high"
)

// SeverityToSignalSeverity maps a domain Event severity to SignalSeverity.
func SeverityToSignalSeverity(s Severity) SignalSeverity {
	switch s {
	case SeverityInfo:
		return SignalSeverityLow
	case SeverityWarning:
		return SignalSeverityMedium
	case SeverityAlert, SeverityCritical:
		return SignalSeverityHigh
	default:
		return ""
	}
}

// SeverityToScore returns a numeric score for the given SignalSeverity.
func SeverityToScore(severity SignalSeverity) float64 {
	switch severity {
	case SignalSeverityLow:
		return 0.33
	case SignalSeverityMedium:
		return 0.66
	case SignalSeverityHigh:
		return 1.0
	default:
		return 0.0
	}
}
