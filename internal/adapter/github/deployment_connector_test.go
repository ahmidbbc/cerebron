package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/adapter/github"
	"cerebron/internal/config"
	"cerebron/internal/port/outbound"
)

func TestDeploymentConnector_Name(t *testing.T) {
	c := github.NewDeploymentConnector(config.ProviderConfig{BaseURL: "http://example.com"})
	if c.Name() != "github" {
		t.Fatalf("expected github, got %s", c.Name())
	}
}

func TestDeploymentConnector_FetchDeployments_Success(t *testing.T) {
	payload := []map[string]any{
		{
			"id":          float64(1),
			"sha":         "abc123",
			"ref":         "main",
			"task":        "my-service",
			"environment": "production",
			"description": "deploy",
			"created_at":  time.Now().UTC().Format(time.RFC3339),
			"updated_at":  time.Now().UTC().Format(time.RFC3339),
			"creator":     map[string]any{"login": "alice"},
			"url":         "https://github.com/deploy/1",
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := github.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL, Token: "tok"})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Source != "github" {
		t.Errorf("unexpected source: %s", deps[0].Source)
	}
	if deps[0].Commit != "abc123" {
		t.Errorf("unexpected commit: %s", deps[0].Commit)
	}
}

func TestDeploymentConnector_FetchDeployments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := github.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL})
	_, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
