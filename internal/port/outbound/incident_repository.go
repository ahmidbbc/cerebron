package outbound

import (
	"context"

	"cerebron/internal/domain"
)

// IncidentRepository persists and retrieves stored incidents.
type IncidentRepository interface {
	// Save stores an incident. If an incident with the same fingerprint
	// already exists its recurrence_count is incremented and it is returned.
	Save(ctx context.Context, incident domain.StoredIncident) (domain.StoredIncident, error)

	// FindByFingerprint returns the stored incident matching the fingerprint,
	// or (zero, false, nil) when not found.
	FindByFingerprint(ctx context.Context, fingerprint string) (domain.StoredIncident, bool, error)

	// ListByService returns all stored incidents for a service, newest first.
	ListByService(ctx context.Context, service string) ([]domain.StoredIncident, error)
}
