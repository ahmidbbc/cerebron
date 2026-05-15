package kubernetes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/adapter/kubernetes"
	"cerebron/internal/config"
	"cerebron/internal/port/outbound"
)

func TestDeploymentConnector_Name(t *testing.T) {
	c := kubernetes.NewDeploymentConnector(config.KubernetesConfig{}, "http://example.com", "")
	if c.Name() != "kubernetes" {
		t.Fatalf("expected kubernetes, got %s", c.Name())
	}
}

func TestDeploymentConnector_FetchDeployments_Success(t *testing.T) {
	payload := map[string]any{
		"items": []map[string]any{
			{
				"metadata": map[string]any{
					"name":              "my-app-v2",
					"namespace":         "production",
					"creationTimestamp": time.Now().UTC().Format(time.RFC3339),
					"annotations": map[string]any{
						"deployment.kubernetes.io/revision": "2",
						"kubernetes.io/change-cause":        "v1.2.0",
					},
					"labels": map[string]any{"app": "my-app"},
				},
				"status": map[string]any{"replicas": float64(3)},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := kubernetes.NewDeploymentConnector(
		config.KubernetesConfig{Namespaces: []string{"production"}},
		srv.URL,
		"tok",
	)
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Source != "kubernetes" {
		t.Errorf("unexpected source: %s", deps[0].Source)
	}
	if deps[0].Service != "my-app" {
		t.Errorf("unexpected service: %s", deps[0].Service)
	}
}

func TestDeploymentConnector_FetchDeployments_SkipsZeroReplicas(t *testing.T) {
	payload := map[string]any{
		"items": []map[string]any{
			{
				"metadata": map[string]any{
					"name":              "my-app-v1",
					"namespace":         "default",
					"creationTimestamp": time.Now().UTC().Format(time.RFC3339),
					"annotations":       map[string]any{},
					"labels":            map[string]any{"app": "my-app"},
				},
				"status": map[string]any{"replicas": float64(0)},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := kubernetes.NewDeploymentConnector(config.KubernetesConfig{}, srv.URL, "")
	deps, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("expected 0 deployments (zero replicas skipped), got %d", len(deps))
	}
}

func TestDeploymentConnector_FetchDeployments_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := kubernetes.NewDeploymentConnector(config.KubernetesConfig{}, srv.URL, "")
	_, err := c.FetchDeployments(context.Background(), outbound.DeploymentQuery{})
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
}
