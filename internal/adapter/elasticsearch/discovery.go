package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// fallbackIndexPatterns are tried in order when the configured pattern resolves to no indices.
var fallbackIndexPatterns = []string{
	"logs-*",
	"filebeat-*",
	"k8s-*",
	"*-logs-*",
	"*",
}

// IndexDiscovery probes an Elasticsearch cluster to find usable indices and service fields.
type IndexDiscovery struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewIndexDiscovery creates an IndexDiscovery for the given cluster.
func NewIndexDiscovery(baseURL, token string, httpClient *http.Client) IndexDiscovery {
	return IndexDiscovery{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

// DiscoverIndices returns indices matching pattern. If the pattern resolves to nothing,
// it tries each fallback pattern and returns the first non-empty match.
// Returns the resolved pattern string suitable for use as an index pattern in search requests.
func (d IndexDiscovery) DiscoverIndices(ctx context.Context, pattern string) (string, error) {
	normalized := strings.Trim(strings.TrimSpace(pattern), "/")
	if normalized == "" {
		normalized = "*"
	}

	found, err := d.catIndices(ctx, normalized)
	if err != nil {
		return "", err
	}
	if len(found) > 0 {
		return normalized, nil
	}

	var lastErr error
	for _, fallback := range fallbackIndexPatterns {
		if fallback == normalized {
			continue
		}
		found, err = d.catIndices(ctx, fallback)
		if err != nil {
			if lastErr == nil {
				lastErr = err
			}
			continue
		}
		if len(found) > 0 {
			return fallback, nil
		}
	}

	if lastErr != nil {
		return normalized, fmt.Errorf("elasticsearch: no indices found for pattern %q; fallback also failed: %w", pattern, lastErr)
	}
	return normalized, fmt.Errorf("elasticsearch: no indices found for pattern %q and all fallbacks exhausted", pattern)
}

// DetectServiceFields probes a sample document from the given index pattern and returns
// the subset of candidate fields that actually appear in the data.
// Falls back to defaultServiceFields if detection fails or returns nothing useful.
func (d IndexDiscovery) DetectServiceFields(ctx context.Context, indexPattern string) ([]string, error) {
	sample, err := d.sampleDocument(ctx, indexPattern)
	if err != nil {
		return nil, err
	}
	if len(sample) == 0 {
		return defaultServiceFields, nil
	}

	present := make([]string, 0, len(defaultServiceFields))
	for _, field := range defaultServiceFields {
		if _, ok := nestedValue(sample, field); ok {
			present = append(present, field)
		}
	}

	if len(present) == 0 {
		return defaultServiceFields, nil
	}

	return present, nil
}

func (d IndexDiscovery) catIndices(ctx context.Context, pattern string) ([]string, error) {
	url := d.baseURL + "/_cat/indices/" + pattern + "?h=index&format=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build cat indices request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if auth := normalizeAuthorizationHeader(d.token); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cat indices request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cat indices returned HTTP %d for pattern %q", resp.StatusCode, pattern)
	}

	var rows []struct {
		Index string `json:"index"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode cat indices response: %w", err)
	}

	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Index != "" {
			names = append(names, r.Index)
		}
	}
	return names, nil
}

func (d IndexDiscovery) sampleDocument(ctx context.Context, indexPattern string) (map[string]any, error) {
	url := d.baseURL + "/" + strings.Trim(indexPattern, "/") + "/_search"
	body := strings.NewReader(`{"size":1,"query":{"match_all":{}}}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("build sample request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if auth := normalizeAuthorizationHeader(d.token); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sample request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sample search returned HTTP %d", resp.StatusCode)
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode sample response: %w", err)
	}

	if len(result.Hits.Hits) == 0 {
		return nil, nil
	}
	return result.Hits.Hits[0].Source, nil
}
