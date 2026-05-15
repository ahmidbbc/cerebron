package detectincidenttrends

import (
	"context"
	"fmt"
	"sort"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

// Input parameters for trend detection. An empty Input returns trends for all services.
type Input struct {
	// Service filters results to a single service when set.
	Service string
}

type Service struct {
	repo outbound.IncidentRepository
}

func NewService(repo outbound.IncidentRepository) Service {
	return Service{repo: repo}
}

func (s Service) Run(ctx context.Context, input Input) (domain.IncidentTrends, error) {
	var (
		incidents []domain.StoredIncident
		err       error
	)

	if input.Service != "" {
		incidents, err = s.repo.ListByService(ctx, input.Service)
	} else {
		incidents, err = s.repo.ListAll(ctx)
	}
	if err != nil {
		return domain.IncidentTrends{}, fmt.Errorf("list incidents: %w", err)
	}

	return computeTrends(incidents), nil
}

// computeTrends derives per-service trend signals from a flat list of stored incidents.
func computeTrends(incidents []domain.StoredIncident) domain.IncidentTrends {
	if len(incidents) == 0 {
		return domain.IncidentTrends{Services: []domain.ServiceTrend{}}
	}

	type bucket struct {
		incidents []domain.StoredIncident
		first     time.Time
		last      time.Time
	}

	byService := make(map[string]*bucket)
	for _, inc := range incidents {
		b, ok := byService[inc.Service]
		if !ok {
			b = &bucket{first: inc.CreatedAt, last: inc.CreatedAt}
			byService[inc.Service] = b
		}
		b.incidents = append(b.incidents, inc)
		if inc.CreatedAt.Before(b.first) {
			b.first = inc.CreatedAt
		}
		if inc.CreatedAt.After(b.last) {
			b.last = inc.CreatedAt
		}
	}

	var (
		trends          []domain.ServiceTrend
		degradingCount  int
		stableCount     int
		improvingCount  int
		globalFirst     time.Time
		globalLast      time.Time
	)

	first := true
	for service, b := range byService {
		totalRecurrence := 0
		for _, inc := range b.incidents {
			totalRecurrence += inc.RecurrenceCount
		}

		span := b.last.Sub(b.first)
		days := span.Hours() / 24
		if days < 1 {
			days = 1
		}
		freqPerDay := float64(len(b.incidents)) / days

		// Sort newest-first within the bucket so severityTrend's half-split is correct.
		sort.Slice(b.incidents, func(i, j int) bool {
			return b.incidents[i].CreatedAt.After(b.incidents[j].CreatedAt)
		})
		dominant := dominantSeverity(b.incidents)
		sevTrend := severityTrend(b.incidents)

		switch sevTrend {
		case domain.TrendDirectionWorsening:
			degradingCount++
		case domain.TrendDirectionImproving:
			improvingCount++
		default:
			stableCount++
		}

		if first || b.first.Before(globalFirst) {
			globalFirst = b.first
		}
		if first || b.last.After(globalLast) {
			globalLast = b.last
		}
		first = false

		trends = append(trends, domain.ServiceTrend{
			Service:         service,
			IncidentCount:   len(b.incidents),
			RecurrenceTotal: totalRecurrence,
			FrequencyPerDay: freqPerDay,
			DominantSeverity: dominant,
			SeverityTrend:   sevTrend,
			FirstSeen:       b.first,
			LastSeen:        b.last,
		})
	}

	sort.Slice(trends, func(i, j int) bool {
		if trends[i].FrequencyPerDay != trends[j].FrequencyPerDay {
			return trends[i].FrequencyPerDay > trends[j].FrequencyPerDay
		}
		return trends[i].Service < trends[j].Service
	})

	observationDays := globalLast.Sub(globalFirst).Hours() / 24
	if observationDays < 1 {
		observationDays = 1
	}

	return domain.IncidentTrends{
		Services:        trends,
		DegradingCount:  degradingCount,
		StableCount:     stableCount,
		ImprovingCount:  improvingCount,
		ObservationDays: observationDays,
	}
}

// dominantSeverity returns the most frequent highest_severity across stored incidents.
func dominantSeverity(incidents []domain.StoredIncident) domain.SignalSeverity {
	counts := make(map[domain.SignalSeverity]int)
	for _, inc := range incidents {
		for _, g := range inc.Analysis.Groups {
			counts[g.HighestSeverity]++
		}
	}
	var best domain.SignalSeverity
	var bestCount int
	for sev, count := range counts {
		if count > bestCount || (count == bestCount && domain.SeverityToScore(sev) > domain.SeverityToScore(best)) {
			best = sev
			bestCount = count
		}
	}
	if best == "" {
		return domain.SignalSeverityLow
	}
	return best
}

// severityTrend compares average severity score of the older half vs newer half of incidents.
// Requires incidents sorted newest-first (as returned by the repo).
func severityTrend(incidents []domain.StoredIncident) domain.TrendDirection {
	if len(incidents) < 2 {
		return domain.TrendDirectionStable
	}

	// incidents are newest-first; older half is the tail.
	mid := len(incidents) / 2
	newer := incidents[:mid]
	older := incidents[mid:]

	olderScore := avgSeverityScore(older)
	newerScore := avgSeverityScore(newer)

	const threshold = 0.15
	switch {
	case newerScore-olderScore > threshold:
		return domain.TrendDirectionWorsening
	case olderScore-newerScore > threshold:
		return domain.TrendDirectionImproving
	default:
		return domain.TrendDirectionStable
	}
}

func avgSeverityScore(incidents []domain.StoredIncident) float64 {
	if len(incidents) == 0 {
		return 0
	}
	total := 0.0
	count := 0
	for _, inc := range incidents {
		for _, g := range inc.Analysis.Groups {
			total += domain.SeverityToScore(g.HighestSeverity)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}
