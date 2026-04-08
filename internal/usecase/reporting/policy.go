package reporting

import "cerebron/internal/domain"

type Policy struct {
	MinimumScore int
}

func DefaultPolicy() Policy {
	return Policy{MinimumScore: 50}
}

func (p Policy) ShouldReport(events []domain.Event) bool {
	return p.Score(events) >= p.MinimumScore
}

func (p Policy) Score(events []domain.Event) int {
	score := 0

	for _, event := range events {
		switch event.Severity {
		case domain.SeverityCritical:
			score += 100
		case domain.SeverityAlert:
			score += 70
		case domain.SeverityWarning:
			score += 40
		case domain.SeverityInfo:
			score += 10
		}

		if event.OwnerTeam != "" {
			score += 5
		}
		if len(event.ChangeRefs) > 0 {
			score += 10
		}
		if event.Service != "" && event.Environment != "" {
			score += 5
		}
		if len(event.CorrelatedIDs) > 0 {
			score += 20
		}
	}

	return score
}
