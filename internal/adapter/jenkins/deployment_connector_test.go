package jenkins_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/adapter/jenkins"
	"cerebron/internal/config"
	"cerebron/internal/port/outbound"
)

func TestDeploymentConnector_Name(t *testing.T) {
	c := jenkins.NewDeploymentConnector(config.ProviderConfig{BaseURL: "http://example.com"})
	if c.Name() != "jenkins" {
		t.Fatalf("expected jenkins, got %s", c.Name())
	}
}

func TestDeploymentConnector_FetchDeployments_Success(t *testing.T) {
	payload := map[string]any{
		"builds": []map[string]any{
			{
				"id":        "42",
				"number":    float64(42),
				"result":    "SUCCESS",
				"timestamp": float64(time.Now().Add(-5*time.Minute).UnixMilli()),
				"duration":  float64(60000),
				"url":       "http://jenkins/job/svc/42/",
				"actions":   []any{},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := jenkins.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL, Token: "tok"})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{Service: "svc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Source != "jenkins" {
		t.Errorf("unexpected source: %s", deps[0].Source)
	}
}

func TestDeploymentConnector_FetchDeployments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := jenkins.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL})
	_, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{Service: "svc"})
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

func TestDeploymentConnector_FetchDeployments_EmptyService(t *testing.T) {
	c := jenkins.NewDeploymentConnector(config.ProviderConfig{BaseURL: "http://example.com"})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error for empty service: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("expected 0 deployments for empty service, got %d", len(deps))
	}
}
