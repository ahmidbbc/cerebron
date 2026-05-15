package jenkins

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

const providerName = "jenkins"

// DeploymentConnector fetches build history from Jenkins and treats successful builds as deployments.
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
	if query.Service == "" {
		return nil, nil
	}
	jobPath := "job/" + url.PathEscape(query.Service)

	endpoint, err := url.Parse(fmt.Sprintf("%s/%s/api/json", c.baseURL, jobPath))
	if err != nil {
		return nil, fmt.Errorf("build builds endpoint: %w", err)
	}

	q := endpoint.Query()
	q.Set("tree", "builds[id,number,result,timestamp,duration,url,actions[parameters[name,value]]]")
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
		return nil, fmt.Errorf("unexpected status %d from jenkins", resp.StatusCode)
	}

	var raw jenkinsBuildList
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode builds response: %w", err)
	}

	deployments := make([]domain.Deployment, 0, len(raw.Builds))
	for _, b := range raw.Builds {
		mapped := mapBuild(b, query.Service)
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

type jenkinsBuildList struct {
	Builds []jenkinsBuild `json:"builds"`
}

type jenkinsBuild struct {
	ID        string `json:"id"`
	Number    int64  `json:"number"`
	Result    string `json:"result"`
	Timestamp int64  `json:"timestamp"`
	Duration  int64  `json:"duration"`
	URL       string `json:"url"`
	Actions   []struct {
		Parameters []struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		} `json:"parameters,omitempty"`
	} `json:"actions"`
}

func mapBuild(b jenkinsBuild, service string) domain.Deployment {
	startedAt := time.Unix(b.Timestamp/1000, 0).UTC()
	finishedAt := startedAt.Add(time.Duration(b.Duration) * time.Millisecond)

	branch := extractParameter(b, "BRANCH")
	commit := extractParameter(b, "GIT_COMMIT")
	env := extractParameter(b, "ENVIRONMENT")

	return domain.Deployment{
		ID:          fmt.Sprintf("jenkins:%s", b.ID),
		Source:      providerName,
		Service:     service,
		Environment: env,
		Branch:      branch,
		Commit:      commit,
		Status:      mapResult(b.Result),
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		URL:         b.URL,
	}
}

func extractParameter(b jenkinsBuild, name string) string {
	for _, action := range b.Actions {
		for _, param := range action.Parameters {
			if strings.EqualFold(param.Name, name) {
				if s, ok := param.Value.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func mapResult(result string) domain.DeploymentStatus {
	switch result {
	case "SUCCESS":
		return domain.DeploymentStatusSuccess
	case "FAILURE", "ABORTED":
		return domain.DeploymentStatusFailure
	case "":
		return domain.DeploymentStatusInProgress
	default:
		return domain.DeploymentStatusUnknown
	}
}
