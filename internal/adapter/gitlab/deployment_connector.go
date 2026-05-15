package gitlab

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

const providerName = "gitlab"

// DeploymentConnector fetches deployment events from the GitLab Deployments API.
type DeploymentConnector struct {
	baseURL    string
	token      string
	projectIDs []string
	httpClient *http.Client
}

func NewDeploymentConnector(cfg config.GitLabConfig) DeploymentConnector {
	return DeploymentConnector{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		token:      cfg.Token,
		projectIDs: append([]string(nil), cfg.ProjectIDs...),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c DeploymentConnector) Name() string {
	return providerName
}

func (c DeploymentConnector) FetchDeployments(ctx context.Context, query outbound.DeploymentQuery) ([]domain.Deployment, error) {
	var all []domain.Deployment

	for _, projectID := range c.projectIDs {
		deps, err := c.fetchForProject(ctx, projectID, query)
		if err != nil {
			return nil, fmt.Errorf("project %s: %w", projectID, err)
		}
		all = append(all, deps...)
	}

	return all, nil
}

func (c DeploymentConnector) fetchForProject(ctx context.Context, projectID string, query outbound.DeploymentQuery) ([]domain.Deployment, error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/api/v4/projects/%s/deployments", c.baseURL, url.PathEscape(projectID)))
	if err != nil {
		return nil, fmt.Errorf("build deployments endpoint: %w", err)
	}

	q := endpoint.Query()
	q.Set("status", "success")
	if query.Environment != "" {
		q.Set("environment", query.Environment)
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from gitlab deployments", resp.StatusCode)
	}

	var raw []gitlabDeployment
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode deployments response: %w", err)
	}

	deployments := make([]domain.Deployment, 0, len(raw))
	for _, d := range raw {
		mapped := mapDeployment(d, projectID)
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

type gitlabDeployment struct {
	ID          int64     `json:"id"`
	IID         int64     `json:"iid"`
	Ref         string    `json:"ref"`
	SHA         string    `json:"sha"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Environment struct {
		Name string `json:"name"`
	} `json:"environment"`
	Deployer struct {
		Name string `json:"name"`
	} `json:"deployer"`
}

func mapDeployment(d gitlabDeployment, projectID string) domain.Deployment {
	return domain.Deployment{
		ID:          fmt.Sprintf("gitlab:%d", d.ID),
		Source:      providerName,
		Service:     projectID,
		Environment: d.Environment.Name,
		Version:     d.Ref,
		Commit:      d.SHA,
		Author:      d.Deployer.Name,
		Branch:      d.Ref,
		Status:      mapStatus(d.Status),
		StartedAt:   d.CreatedAt,
		FinishedAt:  d.FinishedAt,
	}
}

func mapStatus(s string) domain.DeploymentStatus {
	switch s {
	case "success":
		return domain.DeploymentStatusSuccess
	case "failed":
		return domain.DeploymentStatusFailure
	case "running":
		return domain.DeploymentStatusInProgress
	default:
		return domain.DeploymentStatusUnknown
	}
}
