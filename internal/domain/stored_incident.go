package domain

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

type StoredIncident struct {
	ID               string           `json:"id"`
	Fingerprint      string           `json:"fingerprint"`
	Service          string           `json:"service"`
	Analysis         IncidentAnalysis `json:"analysis"`
	CreatedAt        time.Time        `json:"created_at"`
	RecurrenceCount  int              `json:"recurrence_count"`
}

// ComputeFingerprint produces a deterministic hash from the analysis's
// service, model version, and the sorted set of signal summaries.
// Two analyses covering the same failure pattern yield the same fingerprint.
func ComputeFingerprint(a IncidentAnalysis) string {
	summaries := make([]string, 0, len(a.Groups))
	for _, g := range a.Groups {
		for _, s := range g.Signals {
			summaries = append(summaries, s.Source+"|"+s.Service+"|"+s.Summary)
		}
	}
	sort.Strings(summaries)

	key := strings.Join([]string{
		a.Service,
		a.ModelVersion,
		strings.Join(summaries, ";"),
	}, ":")

	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:16])
}
