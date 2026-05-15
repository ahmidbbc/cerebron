package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const providerName = "kubernetes"

// DeploymentConnector fetches rollout history from the Kubernetes API by reading
// ReplicaSet annotations on Deployments. It calls the in-cluster or configured API server
// via plain HTTP using a bearer token (standard service account auth pattern).
type DeploymentConnector struct {
	baseURL    string
	token      string
	namespaces []string
	httpClient *http.Client
}

func NewDeploymentConnector(cfg config.KubernetesConfig, apiServerURL, token string) DeploymentConnector {
	return DeploymentConnector{
		baseURL:    strings.TrimRight(apiServerURL, "/"),
		token:      token,
		namespaces: append([]string(nil), cfg.Namespaces...),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c DeploymentConnector) Name() string {
	return providerName
}

func (c DeploymentConnector) FetchDeployments(ctx context.Context, query outbound.DeploymentQuery) ([]domain.Deployment, error) {
	namespaces := c.namespaces
	if len(namespaces) == 0 {
		namespaces = []string{"default"}
	}

	var all []domain.Deployment
	for _, ns := range namespaces {
		deps, err := c.fetchForNamespace(ctx, ns, query)
		if err != nil {
			return nil, fmt.Errorf("namespace %s: %w", ns, err)
		}
		all = append(all, deps...)
	}

	return all, nil
}

func (c DeploymentConnector) fetchForNamespace(ctx context.Context, namespace string, query outbound.DeploymentQuery) ([]domain.Deployment, error) {
	path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/replicasets", url.PathEscape(namespace))
	endpoint, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("build replicasets endpoint: %w", err)
	}

	q := endpoint.Query()
	if query.Service != "" {
		q.Set("labelSelector", fmt.Sprintf("app=%s", query.Service))
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from kubernetes api", resp.StatusCode)
	}

	var raw k8sReplicaSetList
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode replicasets response: %w", err)
	}

	deployments := make([]domain.Deployment, 0, len(raw.Items))
	for _, rs := range raw.Items {
		if rs.Status.Replicas == 0 {
			continue
		}
		mapped := mapReplicaSet(rs, namespace)
		if !query.Since.IsZero() && mapped.StartedAt.Before(query.Since) {
			continue
		}
		if !query.Until.IsZero() && mapped.StartedAt.After(query.Until) {
			continue
		}
		deployments = append(deployments, mapped)
	}

	return deployments, nil
}

type k8sReplicaSetList struct {
	Items []k8sReplicaSet `json:"items"`
}

type k8sReplicaSet struct {
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		CreationTimestamp time.Time         `json:"creationTimestamp"`
		Annotations       map[string]string `json:"annotations"`
		Labels            map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Replicas int `json:"replicas"`
	} `json:"status"`
}

func mapReplicaSet(rs k8sReplicaSet, namespace string) domain.Deployment {
	service := rs.Metadata.Labels["app"]
	if service == "" {
		service = rs.Metadata.Name
	}
	commit := rs.Metadata.Annotations["deployment.kubernetes.io/revision"]
	image := rs.Metadata.Annotations["kubernetes.io/change-cause"]

	return domain.Deployment{
		ID:          fmt.Sprintf("kubernetes:%s:%s", namespace, rs.Metadata.Name),
		Source:      providerName,
		Service:     service,
		Environment: namespace,
		Version:     image,
		Commit:      commit,
		Status:      domain.DeploymentStatusSuccess,
		StartedAt:   rs.Metadata.CreationTimestamp,
		FinishedAt:  rs.Metadata.CreationTimestamp,
	}
}
