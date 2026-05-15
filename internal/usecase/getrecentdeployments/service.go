package getrecentdeployments

import (
	"context"
	"fmt"
	"sort"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const defaultDeploymentLimit = 20
const defaultLookbackDuration = 24 * time.Hour

// Input parameters for deployment retrieval.
type Input struct {
	Service     string
	Environment string
	Since       time.Time
	Limit       int
}

// Result holds the deployments returned across all providers.
type Result struct {
	Deployments []domain.Deployment `json:"deployments"`
}

// Service fetches recent deployments from all registered deployment providers.
type Service struct {
	providers []outbound.DeploymentProvider
}

func NewService(providers []outbound.DeploymentProvider) Service {
	return Service{providers: providers}
}

func (s Service) Run(ctx context.Context, input Input) (Result, error) {
	if input.Service == "" {
		return Result{}, fmt.Errorf("service is required")
	}

	since := input.Since
	if since.IsZero() {
		since = time.Now().Add(-defaultLookbackDuration)
	}

	query := outbound.DeploymentQuery{
		Service:     input.Service,
		Environment: input.Environment,
		Since:       since,
		Until:       time.Now(),
	}

	var all []domain.Deployment
	for _, p := range s.providers {
		deployments, err := p.FetchDeployments(ctx, query)
		if err != nil {
			continue
		}
		all = append(all, deployments...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].StartedAt.After(all[j].StartedAt)
	})

	limit := input.Limit
	if limit <= 0 {
		limit = defaultDeploymentLimit
	}
	if len(all) > limit {
		all = all[:limit]
	}

	if all == nil {
		all = []domain.Deployment{}
	}

	return Result{Deployments: all}, nil
}
