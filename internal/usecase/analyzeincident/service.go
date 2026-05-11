package analyzeincident

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const defaultGroupWindow = 5 * time.Minute

type Input struct {
	Services []string
	Since    time.Time
	Until    time.Time
}

type Service struct {
	signalProviders []outbound.SignalProvider
	groupWindow     time.Duration
	now             func() time.Time
}

func NewService(signalProviders []outbound.SignalProvider) Service {
	return Service{
		signalProviders: slices.Clone(signalProviders),
		groupWindow:     defaultGroupWindow,
		now:             time.Now,
	}
}

func (s Service) Run(ctx context.Context, input Input) (domain.IncidentAnalysis, error) {
	services := normalizeServices(input.Services)

	until := input.Until
	if until.IsZero() {
		until = s.now()
	}

	since := input.Since
	if since.IsZero() {
		since = until.Add(-s.groupWindow)
	}
	if since.After(until) {
		return domain.IncidentAnalysis{}, fmt.Errorf("invalid incident window: since %s after until %s", since.Format(time.RFC3339), until.Format(time.RFC3339))
	}

	seen := make(map[string]struct{})
	signals := make([]domain.Signal, 0)
	for _, provider := range s.signalProviders {
		providerSignals, err := provider.CollectSignals(ctx, outbound.CollectSignalsQuery{
			Services: services,
			Since:    since,
			Until:    until,
		})
		if err != nil {
			return domain.IncidentAnalysis{}, fmt.Errorf("collect signals from %s: %w", provider.Name(), err)
		}

		signals = append(signals, filterSignals(providerSignals, services, since, until, seen)...)
	}

	sortSignals(signals)
	groups := buildSignalGroups(signals, s.groupWindow)
	metadata := buildMetadata(signals)
	serviceLabel := servicesLabel(services)

	return domain.IncidentAnalysis{
		Service:      serviceLabel,
		TimeRange:    formatTimeRange(since, until),
		ModelVersion: domain.IncidentAnalysisModelVersion,
		Metadata:     metadata,
		Groups:       groups,
		Summary:      buildAnalysisSummary(serviceLabel, metadata, len(groups)),
		Confidence:   computeConfidence(groups, metadata),
	}, nil
}

func normalizeServices(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		lower := strings.ToLower(t)
		if _, dup := seen[lower]; dup {
			continue
		}
		seen[lower] = struct{}{}
		out = append(out, t)
	}
	return out
}

func servicesLabel(services []string) string {
	if len(services) == 0 {
		return ""
	}
	return strings.Join(services, ",")
}

func matchesAnyService(services []string, actual string) bool {
	if len(services) == 0 {
		return true
	}
	actualLower := strings.ToLower(strings.TrimSpace(actual))
	for _, s := range services {
		if strings.ToLower(strings.TrimSpace(s)) == actualLower {
			return true
		}
	}
	return false
}

func signalFingerprint(sig domain.Signal) string {
	return sig.Source + "|" + sig.Service + "|" + sig.Summary + "|" + sig.Timestamp.UTC().Format(time.RFC3339)
}

func filterSignals(signals []domain.Signal, services []string, since time.Time, until time.Time, seen map[string]struct{}) []domain.Signal {
	filtered := make([]domain.Signal, 0, len(signals))
	for _, signal := range signals {
		if !matchesAnyService(services, signal.Service) {
			continue
		}
		if signal.Timestamp.Before(since) || signal.Timestamp.After(until) {
			continue
		}
		fp := signalFingerprint(signal)
		if _, dup := seen[fp]; dup {
			continue
		}
		seen[fp] = struct{}{}
		filtered = append(filtered, signal)
	}
	return filtered
}

func sortSignals(signals []domain.Signal) {
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Timestamp.Equal(signals[j].Timestamp) {
			if signals[i].Service == signals[j].Service {
				return signals[i].Source < signals[j].Source
			}
			return signals[i].Service < signals[j].Service
		}
		return signals[i].Timestamp.Before(signals[j].Timestamp)
	})
}

func deduplicateSignals(signals []domain.Signal) []domain.Signal {
	if len(signals) == 0 {
		return signals
	}
	out := make([]domain.Signal, 0, len(signals))
	for _, sig := range signals {
		key := sig.Source + "|" + sig.Service + "|" + sig.Summary + "|" + string(sig.Severity)
		found := false
		for i := range out {
			existing := out[i].Source + "|" + out[i].Service + "|" + out[i].Summary + "|" + string(out[i].Severity)
			if existing == key {
				out[i].Count++
				found = true
				break
			}
		}
		if !found {
			s := sig
			s.Count = 1
			out = append(out, s)
		}
	}
	return out
}

func buildSignalGroups(signals []domain.Signal, groupWindow time.Duration) []domain.SignalGroup {
	if len(signals) == 0 {
		return nil
	}

	grouped := make(map[time.Time][]domain.Signal)
	for _, signal := range signals {
		windowStart := signal.Timestamp.UTC().Truncate(groupWindow)
		grouped[windowStart] = append(grouped[windowStart], signal)
	}

	windowStarts := make([]time.Time, 0, len(grouped))
	for windowStart := range grouped {
		windowStarts = append(windowStarts, windowStart)
	}
	sort.Slice(windowStarts, func(i, j int) bool {
		return windowStarts[i].Before(windowStarts[j])
	})

	groups := make([]domain.SignalGroup, 0, len(windowStarts))
	for _, windowStart := range windowStarts {
		groupSignals := grouped[windowStart]
		sortSignals(groupSignals)
		groupSignals = deduplicateSignals(groupSignals)
		windowEnd := windowStart.Add(groupWindow)
		groupServiceLabel := distinctServicesLabel(groupSignals)

		groups = append(groups, domain.SignalGroup{
			Service:         groupServiceLabel,
			WindowStart:     windowStart,
			WindowEnd:       windowEnd,
			SourceCount:     countDistinctSources(groupSignals),
			HighestSeverity: highestSeverity(groupSignals),
			Summary:         buildGroupSummary(groupServiceLabel, windowStart, windowEnd, groupSignals),
			Signals:         groupSignals,
		})
	}

	return groups
}

func distinctServicesLabel(signals []domain.Signal) string {
	seen := make(map[string]struct{}, len(signals))
	out := make([]string, 0, len(signals))
	for _, s := range signals {
		t := strings.TrimSpace(s.Service)
		lower := strings.ToLower(t)
		if _, dup := seen[lower]; dup {
			continue
		}
		seen[lower] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func buildMetadata(signals []domain.Signal) domain.Metadata {
	return domain.Metadata{
		TotalSignals:    len(signals),
		DistinctSources: countDistinctSources(signals),
	}
}

func buildAnalysisSummary(serviceLabel string, metadata domain.Metadata, groupCount int) string {
	label := serviceLabel
	if label == "" {
		label = "all services"
	}
	if metadata.TotalSignals == 0 {
		return fmt.Sprintf("No incident signals detected for %s.", label)
	}
	return fmt.Sprintf(
		"%s has %d signals grouped into %d windows across %d distinct sources.",
		label,
		metadata.TotalSignals,
		groupCount,
		metadata.DistinctSources,
	)
}

func buildGroupSummary(serviceLabel string, windowStart time.Time, windowEnd time.Time, signals []domain.Signal) string {
	return fmt.Sprintf(
		"%s has %d signals between %s and %s from %d sources; highest severity is %s.",
		serviceLabel,
		len(signals),
		windowStart.Format(time.RFC3339),
		windowEnd.Format(time.RFC3339),
		countDistinctSources(signals),
		highestSeverity(signals),
	)
}

func highestSeverity(signals []domain.Signal) domain.SignalSeverity {
	var highest domain.SignalSeverity
	highestScore := -1.0

	for _, signal := range signals {
		score := domain.SeverityToScore(signal.Severity)
		if score > highestScore {
			highest = signal.Severity
			highestScore = score
		}
	}

	return highest
}

func countDistinctSources(signals []domain.Signal) int {
	if len(signals) == 0 {
		return 0
	}

	sources := make(map[string]struct{}, len(signals))
	for _, signal := range signals {
		source := strings.TrimSpace(signal.Source)
		if source == "" {
			continue
		}
		sources[source] = struct{}{}
	}

	return len(sources)
}

func computeConfidence(groups []domain.SignalGroup, metadata domain.Metadata) float64 {
	severityScore := weightedSeverityScore(groups, metadata.TotalSignals)
	signalVolumeScore := minFloat64(float64(metadata.TotalSignals)/10.0, 1.0)
	sourceDiversityScore := minFloat64(float64(metadata.DistinctSources)/3.0, 1.0)

	confidence := clampFloat64(
		severityScore*0.5+
			signalVolumeScore*0.3+
			sourceDiversityScore*0.2,
		0,
		1,
	)

	return math.Round(confidence*100) / 100
}

func weightedSeverityScore(groups []domain.SignalGroup, totalSignals int) float64 {
	if totalSignals == 0 {
		return 0
	}

	total := 0.0
	for _, group := range groups {
		total += domain.SeverityToScore(group.HighestSeverity) * float64(len(group.Signals))
	}

	return total / float64(totalSignals)
}

func formatTimeRange(since time.Time, until time.Time) string {
	return since.UTC().Format(time.RFC3339) + "/" + until.UTC().Format(time.RFC3339)
}

func clampFloat64(value float64, minValue float64, maxValue float64) float64 {
	return minFloat64(maxFloat64(value, minValue), maxValue)
}

func minFloat64(left float64, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func maxFloat64(left float64, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
