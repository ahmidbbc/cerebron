package getservicedependencies

import (
	"context"
	"fmt"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

// Input parameters for dependency graph queries.
type Input struct {
	// Service is the focal service. When empty, all edges are returned without blast-radius computation.
	Service string
}

type Service struct {
	providers []outbound.DependencyGraphProvider
}

func NewService(providers []outbound.DependencyGraphProvider) Service {
	return Service{providers: providers}
}

func (s Service) Run(ctx context.Context, input Input) (domain.ServiceDependencies, error) {
	var all []domain.DependencyEdge
	for _, p := range s.providers {
		edges, err := p.FetchDependencies(ctx)
		if err != nil {
			return domain.ServiceDependencies{}, fmt.Errorf("provider %s: %w", p.Name(), err)
		}
		all = append(all, edges...)
	}
	all = dedup(all)
	return buildGraph(input.Service, all), nil
}

// buildGraph derives upstreams, downstreams, and blast radius for the focal service.
func buildGraph(service string, edges []domain.DependencyEdge) domain.ServiceDependencies {
	upSet := make(map[string]struct{})
	downSet := make(map[string]struct{})

	for _, e := range edges {
		if service == "" {
			continue
		}
		if e.Target == service {
			upSet[e.Source] = struct{}{}
		}
		if e.Source == service {
			downSet[e.Target] = struct{}{}
		}
	}

	upstreams := setToSlice(upSet)
	downstreams := setToSlice(downSet)
	blastRadius := computeBlastRadius(service, edges)

	return domain.ServiceDependencies{
		Service:     service,
		Upstreams:   upstreams,
		Downstreams: downstreams,
		BlastRadius: blastRadius,
		AllEdges:    edges,
	}
}

// computeBlastRadius returns all services that transitively depend on the focal service
// (i.e., services that would be impacted if the focal service fails).
func computeBlastRadius(service string, edges []domain.DependencyEdge) []string {
	if service == "" {
		return []string{}
	}
	// Build reverse adjacency: for each node, who calls it?
	// blast radius = transitive set of callers of `service`.
	callers := make(map[string][]string) // target -> sources
	for _, e := range edges {
		callers[e.Target] = append(callers[e.Target], e.Source)
	}

	visited := make(map[string]struct{})
	queue := []string{service}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, caller := range callers[cur] {
			if _, seen := visited[caller]; !seen {
				visited[caller] = struct{}{}
				queue = append(queue, caller)
			}
		}
	}

	return setToSlice(visited)
}

func dedup(edges []domain.DependencyEdge) []domain.DependencyEdge {
	seen := make(map[domain.DependencyEdge]struct{}, len(edges))
	out := make([]domain.DependencyEdge, 0, len(edges))
	for _, e := range edges {
		if _, ok := seen[e]; !ok {
			seen[e] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}

func setToSlice(s map[string]struct{}) []string {
	if len(s) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	return out
}
