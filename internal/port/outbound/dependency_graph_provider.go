package outbound

import (
	"context"

	"cerebron/internal/domain"
)

// DependencyGraphProvider returns service dependency edges from an external source.
type DependencyGraphProvider interface {
	Name() string
	FetchDependencies(ctx context.Context) ([]domain.DependencyEdge, error)
}
