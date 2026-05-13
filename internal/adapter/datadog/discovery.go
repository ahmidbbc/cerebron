package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"cerebron/internal/config"
)

// ServiceDiscovery probes the Datadog API to find the services and environments
// that have active monitors, removing the need for manual configuration.
type ServiceDiscovery struct {
	baseURLs   []string
	apiKey     string
	appKey     string
	httpClient *http.Client
}

// DiscoveredServices is the result of a service discovery probe.
type DiscoveredServices struct {
	Services     []string
	Environments []string
	Capabilities []ServiceCapability
}

// ServiceCapability describes what Datadog can observe for a single service.
type ServiceCapability struct {
	Service      string
	Environments []string
	MonitorCount int
}

// NewServiceDiscovery creates a ServiceDiscovery using the given Datadog config.
func NewServiceDiscovery(cfg config.DatadogConfig) ServiceDiscovery {
	return ServiceDiscovery{
		baseURLs: buildBaseURLs(cfg.BaseURL),
		apiKey:   cfg.APIKey,
		appKey:   cfg.AppKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// DiscoverServices queries Datadog monitors and returns unique services,
// environments, and per-service capability information.
func (d ServiceDiscovery) DiscoverServices(ctx context.Context) (DiscoveredServices, error) {
	monitors, err := d.listMonitors(ctx)
	if err != nil {
		return DiscoveredServices{}, err
	}

	return buildDiscoveredServices(monitors), nil
}

type discoveryMonitor struct {
	Query string   `json:"query"`
	Tags  []string `json:"tags"`
}

func (d ServiceDiscovery) listMonitors(ctx context.Context) ([]discoveryMonitor, error) {
	var lastErr error

	for _, baseURL := range d.baseURLs {
		endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/v1/monitor")
		if err != nil {
			return nil, fmt.Errorf("build monitors endpoint: %w", err)
		}

		q := endpoint.Query()
		// "all" is intentional: discovery wants every monitor regardless of alert state,
		// not just actively firing ones.
		q.Set("group_states", "all")
		endpoint.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("DD-API-KEY", d.apiKey)
		req.Header.Set("DD-APPLICATION-KEY", d.appKey)

		resp, err := d.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("perform request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, baseURL)
			if shouldRetryWithFallback(resp.StatusCode) {
				continue
			}
			return nil, lastErr
		}

		defer resp.Body.Close()
		var payload []discoveryMonitor
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode monitors response: %w", err)
		}
		return payload, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no datadog base url configured")
}

func buildDiscoveredServices(monitors []discoveryMonitor) DiscoveredServices {
	serviceEnvs := make(map[string]map[string]struct{})
	serviceCount := make(map[string]int)

	for _, m := range monitors {
		service, environment, _ := extractMetadataFromTags(m.Tags)
		qm := extractMetadataFromQuery(m.Query)
		if service == "" {
			service = qm.Service
		}
		if environment == "" {
			environment = qm.Environment
		}
		if service == "" {
			continue
		}

		if serviceEnvs[service] == nil {
			serviceEnvs[service] = make(map[string]struct{})
		}
		serviceCount[service]++
		if environment != "" {
			serviceEnvs[service][environment] = struct{}{}
		}
	}

	services := make([]string, 0, len(serviceEnvs))
	allEnvs := make(map[string]struct{})
	capabilities := make([]ServiceCapability, 0, len(serviceEnvs))

	for svc, envs := range serviceEnvs {
		services = append(services, svc)

		svcEnvs := make([]string, 0, len(envs))
		for env := range envs {
			svcEnvs = append(svcEnvs, env)
			allEnvs[env] = struct{}{}
		}
		sort.Strings(svcEnvs)

		capabilities = append(capabilities, ServiceCapability{
			Service:      svc,
			Environments: svcEnvs,
			MonitorCount: serviceCount[svc],
		})
	}

	sort.Strings(services)
	sort.Slice(capabilities, func(i, j int) bool {
		return capabilities[i].Service < capabilities[j].Service
	})

	environments := make([]string, 0, len(allEnvs))
	for env := range allEnvs {
		environments = append(environments, env)
	}
	sort.Strings(environments)

	return DiscoveredServices{
		Services:     services,
		Environments: environments,
		Capabilities: capabilities,
	}
}
