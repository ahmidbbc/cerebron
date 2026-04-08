package outbound

import (
	"context"

	"cerebron/internal/domain"
)

type IncidentRepository interface {
	UpsertIncident(ctx context.Context, incident domain.Incident) error
	ListOpenIncidents(ctx context.Context) ([]domain.Incident, error)
}

type EventRepository interface {
	SaveEvents(ctx context.Context, events []domain.Event) error
}
