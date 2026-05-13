package findsimilarincidents

import (
	"context"
	"fmt"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const defaultLimit = 10

type Input struct {
	Fingerprint string
	Service     string
	Limit       int
}

type Result struct {
	ExactMatch *domain.StoredIncident  `json:"exact_match,omitempty"`
	Related    []domain.StoredIncident `json:"related"`
}

type Service struct {
	repo outbound.IncidentRepository
}

func NewService(repo outbound.IncidentRepository) Service {
	return Service{repo: repo}
}

func (s Service) Run(ctx context.Context, input Input) (Result, error) {
	if input.Fingerprint == "" && input.Service == "" {
		return Result{}, fmt.Errorf("fingerprint or service is required")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	var result Result

	if input.Fingerprint != "" {
		stored, found, err := s.repo.FindByFingerprint(ctx, input.Fingerprint)
		if err != nil {
			return Result{}, fmt.Errorf("find by fingerprint: %w", err)
		}
		if found {
			result.ExactMatch = &stored
		}
	}

	if input.Service != "" {
		related, err := s.repo.ListByService(ctx, input.Service)
		if err != nil {
			return Result{}, fmt.Errorf("list by service: %w", err)
		}

		// Exclude exact match from related list to avoid duplication.
		exactID := ""
		if result.ExactMatch != nil {
			exactID = result.ExactMatch.ID
		}

		filtered := make([]domain.StoredIncident, 0, len(related))
		for _, inc := range related {
			if inc.ID == exactID {
				continue
			}
			filtered = append(filtered, inc)
			if len(filtered) >= limit {
				break
			}
		}
		result.Related = filtered
	}

	if result.Related == nil {
		result.Related = []domain.StoredIncident{}
	}

	return result, nil
}
