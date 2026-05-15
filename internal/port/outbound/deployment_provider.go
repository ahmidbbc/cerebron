package outbound

import (
	"context"
	"time"

	"cerebron/internal/domain"
)

// DeploymentQuery describes which deployments to fetch.
type DeploymentQuery struct {
	Service     string
	Environment string
	Since       time.Time
	Until       time.Time
}

// DeploymentProvider fetches recent deployments from an external system.
type DeploymentProvider interface {
	Name() string
	FetchDeployments(ctx context.Context, query DeploymentQuery) ([]domain.Deployment, error)
}
