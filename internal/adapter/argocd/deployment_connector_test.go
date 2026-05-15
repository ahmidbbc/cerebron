package argocd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/adapter/argocd"
	"cerebron/internal/config"
	"cerebron/internal/port/outbound"
)

func TestDeploymentConnector_Name(t *testing.T) {
	c := argocd.NewDeploymentConnector(config.ProviderConfig{BaseURL: "http://example.com"})
	if c.Name() != "argocd" {
		t.Fatalf("expected argocd, got %s", c.Name())
	}
}

func TestDeploymentConnector_FetchDeployments_Success(t *testing.T) {
	payload := map[string]any{
		"items": []map[string]any{
			{
				"metadata": map[string]any{"name": "my-app"},
				"spec": map[string]any{
					"destination": map[string]any{
						"namespace": "production",
						"server":    "https://k8s",
					},
				},
				"status": map[string]any{
					"history": []map[string]any{
						{
							"id":         float64(5),
							"revision":   "abc",
							"deployedAt": time.Now().UTC().Format(time.RFC3339),
							"source":     map[string]any{"repoURL": "https://github.com/org/repo", "targetRevision": "v1.2.3"},
						},
					},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := argocd.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL, Token: "tok"})
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Source != "argocd" {
		t.Errorf("unexpected source: %s", deps[0].Source)
	}
	if deps[0].Service != "my-app" {
		t.Errorf("unexpected service: %s", deps[0].Service)
	}
}

func TestDeploymentConnector_FetchDeployments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := argocd.NewDeploymentConnector(config.ProviderConfig{BaseURL: srv.URL})
	_, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}
