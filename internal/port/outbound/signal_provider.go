package outbound

import (
	"context"
	"time"

	"cerebron/internal/domain"
)

type CollectSignalsQuery struct {
	Services []string
	Since    time.Time
	Until    time.Time
}

type SignalProvider interface {
	Name() string
	CollectSignals(ctx context.Context, query CollectSignalsQuery) ([]domain.Signal, error)
}
