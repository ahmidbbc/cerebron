package gerrit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
	"cerebron/internal/port/outbound"
)

const providerName = "gerrit"

// DeploymentConnector infers deployment events from recently submitted Gerrit changes.
// Gerrit has no native deployment concept; a submitted change is treated as a deployment.
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
	endpoint, err := url.Parse(c.baseURL + "/a/changes/")
	if err != nil {
		return nil, fmt.Errorf("build changes endpoint: %w", err)
	}

	q := endpoint.Query()
	q.Set("q", "status:merged")
	q.Set("o", "CURRENT_REVISION")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.SetBasicAuth("", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from gerrit changes", resp.StatusCode)
	}

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// Gerrit prefixes responses with )]}' to prevent XSSI attacks.
	body := strings.TrimPrefix(string(rawBytes), ")]}'")
	body = strings.TrimLeft(body, "\n\r ")

	var raw []gerritChange
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, fmt.Errorf("decode changes response: %w", err)
	}

	deployments := make([]domain.Deployment, 0, len(raw))
	for _, ch := range raw {
		mapped := mapChange(ch)
		if mapped.StartedAt.IsZero() {
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

	return deployments, nil
}

type gerritChange struct {
	ID              string `json:"id"`
	Project         string `json:"project"`
	Branch          string `json:"branch"`
	Subject         string `json:"subject"`
	Status          string `json:"status"`
	CurrentRevision string `json:"current_revision"`
	Submitted       string `json:"submitted"`
	Owner           struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"owner"`
}

func mapChange(ch gerritChange) domain.Deployment {
	t, _ := time.Parse("2006-01-02 15:04:05.000000000", ch.Submitted)
	return domain.Deployment{
		ID:         fmt.Sprintf("gerrit:%s", ch.ID),
		Source:     providerName,
		Service:    ch.Project,
		Branch:     ch.Branch,
		Commit:     ch.CurrentRevision,
		Author:     ch.Owner.Name,
		Status:     domain.DeploymentStatusSuccess,
		StartedAt:  t,
		FinishedAt: t,
	}
}
