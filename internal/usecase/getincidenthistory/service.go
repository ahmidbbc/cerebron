package getincidenthistory

import (
	"context"
	"fmt"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const defaultHistoryLimit = 50

// Input parameters for incident history retrieval.
type Input struct {
	Service string
	Limit   int
}

// Result holds the stored incidents returned from the repository.
type Result struct {
	Incidents []domain.StoredIncident `json:"incidents"`
	Total     int                     `json:"total"`
}

// Service retrieves incident history from the repository.
type Service struct {
	repo outbound.IncidentRepository
}

func NewService(repo outbound.IncidentRepository) Service {
	return Service{repo: repo}
}

func (s Service) Run(ctx context.Context, input Input) (Result, error) {
	if input.Service == "" {
		return Result{}, fmt.Errorf("service is required")
	}

	incidents, err := s.repo.ListByService(ctx, input.Service)
	if err != nil {
		return Result{}, fmt.Errorf("list by service: %w", err)
	}

	total := len(incidents)

	limit := input.Limit
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if len(incidents) > limit {
		incidents = incidents[:limit]
	}

	if incidents == nil {
		incidents = []domain.StoredIncident{}
	}

	return Result{Incidents: incidents, Total: total}, nil
}
