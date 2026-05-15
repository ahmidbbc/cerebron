package cerebron_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	cerebron "cerebron/sdk/go"
)

func TestClient_AnalyzeIncident(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/incidents/analyze" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cerebron.IncidentAnalysis{
			Service:    "api",
			Confidence: 0.9,
			Summary:    "high error rate",
		})
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	result, err := c.AnalyzeIncident(context.Background(), cerebron.AnalyzeIncidentRequest{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Service != "api" {
		t.Errorf("expected service=api, got %s", result.Service)
	}
	if result.Confidence != 0.9 {
		t.Errorf("expected confidence=0.9, got %f", result.Confidence)
	}
}

func TestClient_APIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"service not found"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	_, err := c.AnalyzeIncident(context.Background(), cerebron.AnalyzeIncidentRequest{Service: "x"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClient_FindSimilarIncidents(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/incidents/similar" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cerebron.SimilarIncidentsResponse{
			Related: []cerebron.StoredIncident{{ID: "i1", Service: "api"}},
		})
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	result, err := c.FindSimilarIncidents(context.Background(), "", "api", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Related) != 1 {
		t.Errorf("expected 1 related incident, got %d", len(result.Related))
	}
}

func TestClient_DetectIncidentTrends(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cerebron.IncidentTrends{DegradingCount: 2})
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	result, err := c.DetectIncidentTrends(context.Background(), "api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DegradingCount != 2 {
		t.Errorf("expected degrading_count=2, got %d", result.DegradingCount)
	}
}

func TestClient_GetRecentDeployments(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cerebron.RecentDeploymentsResponse{
			Deployments: []cerebron.Deployment{{ID: "d1", Service: "api"}},
		})
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	result, err := c.GetRecentDeployments(context.Background(), "api", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Deployments) != 1 {
		t.Errorf("expected 1 deployment, got %d", len(result.Deployments))
	}
}

func TestClient_GetIncidentHistory(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cerebron.IncidentHistoryResponse{
			Incidents: []cerebron.StoredIncident{{ID: "i1"}},
			Total:     1,
		})
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	result, err := c.GetIncidentHistory(context.Background(), "api", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected total=1, got %d", result.Total)
	}
}

func TestClient_GetServiceDependencies(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services/dependencies" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cerebron.ServiceDependencies{
			Service:     "api",
			Upstreams:   []string{"db"},
			Downstreams: []string{"frontend"},
		})
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	result, err := c.GetServiceDependencies(context.Background(), "api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Service != "api" {
		t.Errorf("expected service=api, got %s", result.Service)
	}
	if len(result.Upstreams) != 1 {
		t.Errorf("expected 1 upstream, got %d", len(result.Upstreams))
	}
}

func TestClient_AnalyzeCausalHints(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/incidents/causal-hints" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cerebron.CausalAnalysis{
			Service: "api",
			Hints: []cerebron.CausalHint{
				{Rule: "deployment_triggered", Confidence: 0.8, Evidence: "deploy before spike"},
			},
		})
	}))
	defer srv.Close()

	c := cerebron.NewClient(srv.URL)
	result, err := c.AnalyzeCausalHints(context.Background(), cerebron.IncidentAnalysis{Service: "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Hints) != 1 {
		t.Errorf("expected 1 hint, got %d", len(result.Hints))
	}
}

func TestClient_MCPEndpoint(t *testing.T) {
	t.Parallel()
	c := cerebron.NewClient("http://localhost:8080")
	if c.MCPEndpoint() != "http://localhost:8080/mcp" {
		t.Errorf("unexpected MCP endpoint: %s", c.MCPEndpoint())
	}
}
