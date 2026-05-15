// Package cerebron provides a Go client for the Cerebron operational intelligence API.
// It supports both the MCP transport and a direct HTTP fallback.
package cerebron

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is the Cerebron API client. Use NewClient to construct one.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// NewClient returns a client pointing at baseURL (e.g. "http://localhost:8080").
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// AnalyzeIncidentRequest is the input for AnalyzeIncident.
type AnalyzeIncidentRequest struct {
	Service   string   `json:"service,omitempty"`
	Services  []string `json:"services,omitempty"`
	TimeRange string   `json:"time_range,omitempty"`
}

// Signal is a normalized operational signal.
type Signal struct {
	Source    string            `json:"source"`
	Service   string            `json:"service"`
	Type      string            `json:"type"`
	Summary   string            `json:"summary"`
	Severity  string            `json:"severity"`
	Timestamp time.Time         `json:"timestamp"`
	Count     int               `json:"count,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SignalGroup is a time-windowed cluster of correlated signals.
type SignalGroup struct {
	Service         string    `json:"service"`
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	SourceCount     int       `json:"source_count"`
	HighestSeverity string    `json:"highest_severity"`
	Summary         string    `json:"summary"`
	Signals         []Signal  `json:"signals"`
}

// Deployment represents a deployment event from any CI/CD system.
type Deployment struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Service     string    `json:"service"`
	Environment string    `json:"environment"`
	Version     string    `json:"version"`
	Commit      string    `json:"commit"`
	Author      string    `json:"author"`
	Branch      string    `json:"branch"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	URL         string    `json:"url"`
}

// DeploymentContext holds deployment correlation data attached to an incident analysis.
type DeploymentContext struct {
	RecentDeployments  []Deployment `json:"recent_deployments"`
	SuspectDeployments []Deployment `json:"suspect_deployments"`
	RollbackCandidates []Deployment `json:"rollback_candidates"`
}

// IncidentAnalysis is the response from AnalyzeIncident.
type IncidentAnalysis struct {
	Service           string             `json:"service"`
	TimeRange         string             `json:"time_range"`
	ModelVersion      string             `json:"model_version"`
	Groups            []SignalGroup      `json:"groups"`
	Summary           string             `json:"summary"`
	Confidence        float64            `json:"confidence"`
	DeploymentContext *DeploymentContext `json:"deployment_context,omitempty"`
}

// StoredIncident is a persisted incident record with recurrence tracking.
type StoredIncident struct {
	ID              string           `json:"id"`
	Fingerprint     string           `json:"fingerprint"`
	Service         string           `json:"service"`
	Analysis        IncidentAnalysis `json:"analysis"`
	CreatedAt       time.Time        `json:"created_at"`
	RecurrenceCount int              `json:"recurrence_count"`
}

// SimilarIncidentsResponse is the response from FindSimilarIncidents.
type SimilarIncidentsResponse struct {
	ExactMatch *StoredIncident  `json:"exact_match,omitempty"`
	Related    []StoredIncident `json:"related"`
}

// ServiceTrend aggregates trend signals for a single service.
type ServiceTrend struct {
	Service          string    `json:"service"`
	IncidentCount    int       `json:"incident_count"`
	RecurrenceTotal  int       `json:"recurrence_total"`
	FrequencyPerDay  float64   `json:"frequency_per_day"`
	DominantSeverity string    `json:"dominant_severity"`
	SeverityTrend    string    `json:"severity_trend"`
	FirstSeen        time.Time `json:"first_seen"`
	LastSeen         time.Time `json:"last_seen"`
}

// IncidentTrends is the response from DetectIncidentTrends.
type IncidentTrends struct {
	Services        []ServiceTrend `json:"services"`
	DegradingCount  int            `json:"degrading_count"`
	StableCount     int            `json:"stable_count"`
	ImprovingCount  int            `json:"improving_count"`
	ObservationDays float64        `json:"observation_days"`
}

// DependencyEdge is a directional dependency between two services.
type DependencyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// ServiceDependencies is the response from GetServiceDependencies.
type ServiceDependencies struct {
	Service     string           `json:"service"`
	Upstreams   []string         `json:"upstreams"`
	Downstreams []string         `json:"downstreams"`
	BlastRadius []string         `json:"blast_radius"`
	AllEdges    []DependencyEdge `json:"all_edges"`
}

// CausalHint is a single deterministic causal observation.
type CausalHint struct {
	Rule       string  `json:"rule"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

// CausalAnalysis is the response from AnalyzeCausalHints.
type CausalAnalysis struct {
	Service string       `json:"service"`
	Hints   []CausalHint `json:"hints"`
}

// RecentDeploymentsResponse is the response from GetRecentDeployments.
type RecentDeploymentsResponse struct {
	Deployments []Deployment `json:"deployments"`
}

// IncidentHistoryResponse is the response from GetIncidentHistory.
type IncidentHistoryResponse struct {
	Incidents []StoredIncident `json:"incidents"`
	Total     int              `json:"total"`
}

// MCPEndpoint returns the MCP endpoint URL — pass to any standard MCP client.
func (c *Client) MCPEndpoint() string {
	return c.baseURL + "/mcp"
}

// AnalyzeIncident calls POST /api/v1/incidents/analyze.
func (c *Client) AnalyzeIncident(ctx context.Context, req AnalyzeIncidentRequest) (IncidentAnalysis, error) {
	var result IncidentAnalysis
	err := c.post(ctx, "/api/v1/incidents/analyze", req, &result)
	return result, err
}

// FindSimilarIncidents calls GET /api/v1/incidents/similar.
func (c *Client) FindSimilarIncidents(ctx context.Context, fingerprint, service string, limit int) (SimilarIncidentsResponse, error) {
	params := url.Values{"limit": {strconv.Itoa(limit)}}
	if fingerprint != "" {
		params.Set("fingerprint", fingerprint)
	}
	if service != "" {
		params.Set("service", service)
	}
	var result SimilarIncidentsResponse
	err := c.get(ctx, "/api/v1/incidents/similar?"+params.Encode(), &result)
	return result, err
}

// DetectIncidentTrends calls GET /api/v1/incidents/trends.
func (c *Client) DetectIncidentTrends(ctx context.Context, service string) (IncidentTrends, error) {
	path := "/api/v1/incidents/trends"
	if service != "" {
		path += "?" + url.Values{"service": {service}}.Encode()
	}
	var result IncidentTrends
	err := c.get(ctx, path, &result)
	return result, err
}

// GetServiceDependencies calls GET /api/v1/services/dependencies.
func (c *Client) GetServiceDependencies(ctx context.Context, service string) (ServiceDependencies, error) {
	var result ServiceDependencies
	err := c.get(ctx, "/api/v1/services/dependencies?"+url.Values{"service": {service}}.Encode(), &result)
	return result, err
}

// AnalyzeCausalHints calls POST /api/v1/incidents/causal-hints.
func (c *Client) AnalyzeCausalHints(ctx context.Context, analysis IncidentAnalysis) (CausalAnalysis, error) {
	var result CausalAnalysis
	err := c.post(ctx, "/api/v1/incidents/causal-hints", analysis, &result)
	return result, err
}

// GetRecentDeployments calls GET /api/v1/deployments.
func (c *Client) GetRecentDeployments(ctx context.Context, service, environment string, limit int) (RecentDeploymentsResponse, error) {
	params := url.Values{"service": {service}, "limit": {strconv.Itoa(limit)}}
	if environment != "" {
		params.Set("environment", environment)
	}
	var result RecentDeploymentsResponse
	err := c.get(ctx, "/api/v1/deployments?"+params.Encode(), &result)
	return result, err
}

// GetIncidentHistory calls GET /api/v1/incidents/history.
func (c *Client) GetIncidentHistory(ctx context.Context, service string, limit int) (IncidentHistoryResponse, error) {
	params := url.Values{"service": {service}, "limit": {strconv.Itoa(limit)}}
	var result IncidentHistoryResponse
	err := c.get(ctx, "/api/v1/incidents/history?"+params.Encode(), &result)
	return result, err
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Message != "" {
			return fmt.Errorf("cerebron API error %d: %s", resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("cerebron API error %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
