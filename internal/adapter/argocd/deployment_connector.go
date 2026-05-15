package argocd

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

const providerName = "argocd"

// DeploymentConnector fetches application sync history from the ArgoCD API.
type DeploymentConnector struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewDeploymentConnector(cfg config.ProviderConfig) DeploymentConnector {
	return DeploymentConnector{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.Token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c DeploymentConnector) Name() string {
	return providerName
}

func (c DeploymentConnector) FetchDeployments(ctx context.Context, query outbound.DeploymentQuery) ([]domain.Deployment, error) {
	endpoint, err := url.Parse(c.baseURL + "/api/v1/applications")
	if err != nil {
		return nil, fmt.Errorf("build applications endpoint: %w", err)
	}

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
		return nil, fmt.Errorf("unexpected status %d from argocd", resp.StatusCode)
	}

	var raw argoCDApplicationList
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode applications response: %w", err)
	}

	var deployments []domain.Deployment
	for _, app := range raw.Items {
		if query.Service != "" && app.Metadata.Name != query.Service {
			continue
		}
		if query.Environment != "" && app.Spec.Destination.Namespace != query.Environment {
			continue
		}
		for _, op := range app.Status.History {
			mapped, ok := mapOperation(app, op)
			if !ok {
				continue
			}
			if !query.Since.IsZero() && mapped.StartedAt.Before(query.Since) {
				continue
			}
			if !query.Until.IsZero() && mapped.StartedAt.After(query.Until) {
				continue
			}
			deployments = append(deployments, mapped)
		}
	}

	return deployments, nil
}

type argoCDApplicationList struct {
	Items []argoCDApplication `json:"items"`
}

type argoCDApplication struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Destination struct {
			Namespace string `json:"namespace"`
			Server    string `json:"server"`
		} `json:"destination"`
	} `json:"spec"`
	Status struct {
		History []argoCDOperation `json:"history"`
	} `json:"status"`
}

type argoCDOperation struct {
	ID         int64  `json:"id"`
	Revision   string `json:"revision"`
	DeployedAt string `json:"deployedAt"`
	Source     struct {
		RepoURL        string `json:"repoURL"`
		TargetRevision string `json:"targetRevision"`
	} `json:"source"`
}

func mapOperation(app argoCDApplication, op argoCDOperation) (domain.Deployment, bool) {
	t, err := time.Parse(time.RFC3339, op.DeployedAt)
	if err != nil {
		return domain.Deployment{}, false
	}
	return domain.Deployment{
		ID:          fmt.Sprintf("argocd:%s:%d", app.Metadata.Name, op.ID),
		Source:      providerName,
		Service:     app.Metadata.Name,
		Environment: app.Spec.Destination.Namespace,
		Commit:      op.Revision,
		Version:     op.Source.TargetRevision,
		Status:      domain.DeploymentStatusSuccess,
		StartedAt:   t,
		FinishedAt:  t,
	}, true
}
