package analyzeincident

import (
	"context"
	"io"
	"log/slog"
	"time"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type slowProviderStub struct {
	delay time.Duration
}

func (s *slowProviderStub) Name() string { return "slow" }

func (s *slowProviderStub) CollectSignals(ctx context.Context, _ outbound.CollectSignalsQuery) ([]domain.Signal, error) {
	select {
	case <-time.After(s.delay):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
