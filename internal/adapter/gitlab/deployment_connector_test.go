package gitlab_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/adapter/gitlab"
	"cerebron/internal/config"
	"cerebron/internal/port/outbound"
)

func TestDeploymentConnector_Name(t *testing.T) {
	c := gitlab.NewDeploymentConnector(config.GitLabConfig{ProviderConfig: config.ProviderConfig{BaseURL: "http://example.com"}})
	if c.Name() != "gitlab" {
		t.Fatalf("expected gitlab, got %s", c.Name())
	}
}

func TestDeploymentConnector_FetchDeployments_Success(t *testing.T) {
	payload := []map[string]any{
		{
			"id":     float64(10),
			"ref":    "main",
			"sha":    "deadbeef",
			"status": "success",
			"environment": map[string]any{
				"name": "production",
			},
			"deployer":    map[string]any{"name": "charlie"},
			"created_at":  time.Now().UTC().Format(time.RFC3339),
			"updated_at":  time.Now().UTC().Format(time.RFC3339),
			"finished_at": time.Now().UTC().Format(time.RFC3339),
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := gitlab.NewDeploymentConnector(config.GitLabConfig{
		ProviderConfig: config.ProviderConfig{BaseURL: srv.URL, Token: "tok"},
		ProjectIDs:     []string{"42"},
	})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Source != "gitlab" {
		t.Errorf("unexpected source: %s", deps[0].Source)
	}
}

func TestDeploymentConnector_FetchDeployments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := gitlab.NewDeploymentConnector(config.GitLabConfig{
		ProviderConfig: config.ProviderConfig{BaseURL: srv.URL},
		ProjectIDs:     []string{"1"},
	})
	_, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
}

func TestDeploymentConnector_FetchDeployments_NoProjects(t *testing.T) {
	c := gitlab.NewDeploymentConnector(config.GitLabConfig{
		ProviderConfig: config.ProviderConfig{BaseURL: "http://example.com"},
	})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("expected 0 deployments for no projects, got %d", len(deps))
	}
}
