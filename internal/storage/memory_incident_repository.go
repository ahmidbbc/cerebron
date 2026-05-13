package storage

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"cerebron/internal/domain"
)

// MemoryIncidentRepository is a thread-safe in-memory implementation of
// outbound.IncidentRepository. Suitable for development and testing; replace
// with a Postgres-backed implementation for production persistence.
type MemoryIncidentRepository struct {
	mu            sync.RWMutex
	byID          map[string]domain.StoredIncident
	byFingerprint map[string]string // fingerprint → ID
	counter       int64
	now           func() time.Time
}

func NewMemoryIncidentRepository() *MemoryIncidentRepository {
	return &MemoryIncidentRepository{
		byID:          make(map[string]domain.StoredIncident),
		byFingerprint: make(map[string]string),
		now:           time.Now,
	}
}

func (r *MemoryIncidentRepository) Save(ctx context.Context, incident domain.StoredIncident) (domain.StoredIncident, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id, ok := r.byFingerprint[incident.Fingerprint]; ok {
		existing := r.byID[id]
		existing.RecurrenceCount++
		r.byID[id] = existing
		return existing, nil
	}

	r.counter++
	incident.ID = fmt.Sprintf("inc-%d", r.counter)
	incident.CreatedAt = r.now().UTC()
	incident.RecurrenceCount = 1
	r.byID[incident.ID] = incident
	r.byFingerprint[incident.Fingerprint] = incident.ID
	return incident, nil
}

func (r *MemoryIncidentRepository) FindByFingerprint(ctx context.Context, fingerprint string) (domain.StoredIncident, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byFingerprint[fingerprint]
	if !ok {
		return domain.StoredIncident{}, false, nil
	}
	return r.byID[id], true, nil
}

func (r *MemoryIncidentRepository) ListByService(ctx context.Context, service string) ([]domain.StoredIncident, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.StoredIncident, 0)
	for _, inc := range r.byID {
		if inc.Service == service {
			out = append(out, inc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}
