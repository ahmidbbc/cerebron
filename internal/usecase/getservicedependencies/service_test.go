package getservicedependencies_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
	"cerebron/internal/usecase/getservicedependencies"
)

type stubProvider struct {
	edges []domain.DependencyEdge
	err   error
}

func (s stubProvider) Name() string { return "stub" }
func (s stubProvider) FetchDependencies(_ context.Context) ([]domain.DependencyEdge, error) {
	return s.edges, s.err
}

func sorted(ss []string) []string {
	cp := make([]string, len(ss))
	copy(cp, ss)
	sort.Strings(cp)
	return cp
}

func TestRunReturnsUpstreamsAndDownstreams(t *testing.T) {
	t.Parallel()

	edges := []domain.DependencyEdge{
		{Source: "api", Target: "payments"},
		{Source: "web", Target: "api"},
		{Source: "payments", Target: "db"},
	}
	svc := getservicedependencies.NewService([]outbound.DependencyGraphProvider{stubProvider{edges: edges}})

	result, err := svc.Run(context.Background(), getservicedependencies.Input{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Service != "api" {
		t.Errorf("expected service api, got %s", result.Service)
	}
	if len(result.Upstreams) != 1 || result.Upstreams[0] != "web" {
		t.Errorf("expected upstream [web], got %v", result.Upstreams)
	}
	if len(result.Downstreams) != 1 || result.Downstreams[0] != "payments" {
		t.Errorf("expected downstream [payments], got %v", result.Downstreams)
	}
}

func TestRunComputesBlastRadius(t *testing.T) {
	t.Parallel()

	// db <- payments <- api <- web
	edges := []domain.DependencyEdge{
		{Source: "payments", Target: "db"},
		{Source: "api", Target: "payments"},
		{Source: "web", Target: "api"},
	}
	svc := getservicedependencies.NewService([]outbound.DependencyGraphProvider{stubProvider{edges: edges}})

	result, err := svc.Run(context.Background(), getservicedependencies.Input{Service: "db"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := sorted(result.BlastRadius)
	want := []string{"api", "payments", "web"}
	if len(got) != len(want) {
		t.Fatalf("expected blast radius %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("blast radius mismatch at %d: want %s, got %s", i, want[i], got[i])
		}
	}
}

func TestRunReturnsErrorOnProviderFailure(t *testing.T) {
	t.Parallel()

	svc := getservicedependencies.NewService([]outbound.DependencyGraphProvider{
		stubProvider{err: errors.New("provider down")},
	})

	_, err := svc.Run(context.Background(), getservicedependencies.Input{Service: "api"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunWithEmptyServiceReturnsAllEdgesAndEmptySlices(t *testing.T) {
	t.Parallel()

	edges := []domain.DependencyEdge{
		{Source: "a", Target: "b"},
		{Source: "b", Target: "c"},
	}
	svc := getservicedependencies.NewService([]outbound.DependencyGraphProvider{stubProvider{edges: edges}})

	result, err := svc.Run(context.Background(), getservicedependencies.Input{Service: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Upstreams) != 0 {
		t.Errorf("expected empty upstreams, got %v", result.Upstreams)
	}
	if len(result.Downstreams) != 0 {
		t.Errorf("expected empty downstreams, got %v", result.Downstreams)
	}
	if len(result.BlastRadius) != 0 {
		t.Errorf("expected empty blast radius, got %v", result.BlastRadius)
	}
	if len(result.AllEdges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(result.AllEdges))
	}
}

func TestRunMergesEdgesFromMultipleProviders(t *testing.T) {
	t.Parallel()

	p1 := stubProvider{edges: []domain.DependencyEdge{{Source: "a", Target: "b"}}}
	p2 := stubProvider{edges: []domain.DependencyEdge{{Source: "b", Target: "c"}, {Source: "a", Target: "b"}}}
	svc := getservicedependencies.NewService([]outbound.DependencyGraphProvider{p1, p2})

	result, err := svc.Run(context.Background(), getservicedependencies.Input{Service: "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After dedup: a->b, b->c (2 unique edges)
	if len(result.AllEdges) != 2 {
		t.Errorf("expected 2 unique edges after merge+dedup, got %d", len(result.AllEdges))
	}
	if len(result.Upstreams) != 1 || result.Upstreams[0] != "a" {
		t.Errorf("expected upstream [a], got %v", result.Upstreams)
	}
	if len(result.Downstreams) != 1 || result.Downstreams[0] != "c" {
		t.Errorf("expected downstream [c], got %v", result.Downstreams)
	}
}

func TestRunDeduplicatesEdges(t *testing.T) {
	t.Parallel()

	edges := []domain.DependencyEdge{
		{Source: "a", Target: "b"},
		{Source: "a", Target: "b"},
	}
	svc := getservicedependencies.NewService([]outbound.DependencyGraphProvider{stubProvider{edges: edges}})

	result, err := svc.Run(context.Background(), getservicedependencies.Input{Service: "a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AllEdges) != 1 {
		t.Errorf("expected 1 unique edge, got %d", len(result.AllEdges))
	}
}
