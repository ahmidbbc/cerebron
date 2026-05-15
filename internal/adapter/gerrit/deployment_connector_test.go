package gerrit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cerebron/internal/adapter/gerrit"
	"cerebron/internal/config"
	"cerebron/internal/port/outbound"
)

func TestDeploymentConnector_Name(t *testing.T) {
	c := gerrit.NewDeploymentConnector(config.ProviderConfig{BaseURL: "http://example.com"})
	if c.Name() != "gerrit" {
		t.Fatalf("expected gerrit, got %s", c.Name())
	}
}

func TestDeploymentConnector_FetchDeployments_Success(t *testing.T) {
	changes := []map[string]any{
		{
			"id":               "proj~main~Iabc",
			"project":          "my-service",
			"branch":           "main",
			"subject":          "fix bug",
			"status":           "MERGED",
			"current_revision": "sha1",
			"submitted":        "2024-01-15 10:00:00.000000000",
			"owner":            map[string]any{"name": "bob", "email": "bob@example.com"},
		},
	}
	changesJSON, _ := json.Marshal(changes)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Gerrit anti-XSSI prefix
		w.Write([]byte(")]}'"))
		w.Write([]byte("\n"))
		w.Write(changesJSON)
	}))
	defer srv.Close()

	c := gerrit.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Source != "gerrit" {
		t.Errorf("unexpected source: %s", deps[0].Source)
	}
}

func TestDeploymentConnector_FetchDeployments_PlainJSON(t *testing.T) {
	changes := []map[string]any{
		{
			"id":               "proj~main~Ixyz",
			"project":          "svc",
			"branch":           "main",
			"status":           "MERGED",
			"current_revision": "rev1",
			"submitted":        "2024-03-01 12:00:00.000000000",
			"owner":            map[string]any{"name": "alice", "email": "alice@example.com"},
		},
	}
	changesJSON, _ := json.Marshal(changes)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(changesJSON)
	}))
	defer srv.Close()

	c := gerrit.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
}

func TestDeploymentConnector_FetchDeployments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := gerrit.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL})
	_, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}
