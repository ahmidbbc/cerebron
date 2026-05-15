package domain

// DependencyEdge represents a directional dependency between two services.
type DependencyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// ServiceDependencies holds the dependency graph centered on a single service.
type ServiceDependencies struct {
	Service     string           `json:"service"`
	Upstreams   []string         `json:"upstreams"`
	Downstreams []string         `json:"downstreams"`
	BlastRadius []string         `json:"blast_radius"`
	AllEdges    []DependencyEdge `json:"all_edges"`
}
