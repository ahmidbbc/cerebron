package github

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

const providerName = "github"

// DeploymentConnector fetches deployment events from the GitHub Deployments API.
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
	endpoint, err := url.Parse(c.baseURL + "/deployments")
	if err != nil {
		return nil, fmt.Errorf("build deployments endpoint: %w", err)
	}

	q := endpoint.Query()
	if query.Environment != "" {
		q.Set("environment", query.Environment)
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from github deployments", resp.StatusCode)
	}

	var raw []githubDeployment
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode deployments response: %w", err)
	}

	deployments := make([]domain.Deployment, 0, len(raw))
	for _, d := range raw {
		mapped := mapDeployment(d)
		if !query.Since.IsZero() && mapped.StartedAt.Before(query.Since) {
			continue
		}
		if !query.Until.IsZero() && mapped.StartedAt.After(query.Until) {
			continue
		}
		if query.Service != "" && mapped.Service != query.Service {
			continue
		}
		deployments = append(deployments, mapped)
	}

	return deployments, nil
}

type githubDeployment struct {
	ID          int64     `json:"id"`
	SHA         string    `json:"sha"`
	Ref         string    `json:"ref"`
	Task        string    `json:"task"`
	Environment string    `json:"environment"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Creator     struct {
		Login string `json:"login"`
	} `json:"creator"`
	URL string `json:"url"`
}

func mapDeployment(d githubDeployment) domain.Deployment {
	return domain.Deployment{
		ID:          fmt.Sprintf("github:%d", d.ID),
		Source:      providerName,
		Service:     d.Task,
		Environment: d.Environment,
		Version:     d.Ref,
		Commit:      d.SHA,
		Author:      d.Creator.Login,
		Branch:      d.Ref,
		Status:      domain.DeploymentStatusUnknown,
		StartedAt:   d.CreatedAt,
		FinishedAt:  d.UpdatedAt,
		URL:         d.URL,
	}
}
