package elasticsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestDiscovery(serverURL string) IndexDiscovery {
	return NewIndexDiscovery(serverURL, "test-token", &http.Client{Timeout: 5 * time.Second})
}

func TestDiscoverIndicesReturnsPatternWhenIndicesFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"index": "logs-app-default"},
			{"index": "logs-app-2026"},
		})
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	resolved, err := d.DiscoverIndices(context.Background(), "logs-*")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resolved != "logs-*" {
		t.Fatalf("expected resolved pattern logs-*, got %q", resolved)
	}
}

func TestDiscoverIndicesFallsBackWhenPatternReturnsNothing(t *testing.T) {
	t.Parallel()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// First call (the configured pattern) returns empty; second (filebeat-*) returns data.
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode([]map[string]string{})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"index": "filebeat-2026.01"},
		})
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	resolved, err := d.DiscoverIndices(context.Background(), "custom-nonexistent-*")
	if err != nil {
		t.Fatalf("expected no error after fallback, got %v", err)
	}
	if resolved == "custom-nonexistent-*" {
		t.Fatalf("expected a fallback pattern, still got original pattern")
	}
	if resolved != "logs-*" {
		t.Fatalf("expected first fallback pattern logs-*, got %q", resolved)
	}
}

func TestDiscoverIndicesReturnsErrorWhenAllFallbacksExhausted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	_, err := d.DiscoverIndices(context.Background(), "empty-pattern-*")
	if err == nil {
		t.Fatal("expected error when all fallbacks exhausted")
	}
}

func TestDiscoverIndicesHandles404AsEmpty(t *testing.T) {
	t.Parallel()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{{"index": "logs-app"}})
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	resolved, err := d.DiscoverIndices(context.Background(), "missing-index")
	if err != nil {
		t.Fatalf("404 should trigger fallback, not error; got %v", err)
	}
	if resolved == "missing-index" {
		t.Fatal("expected fallback pattern after 404")
	}
}

func TestDetectServiceFieldsReturnsPresentFields(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"hits": {
				"hits": [{
					"_id": "1",
					"_index": "logs-app",
					"_source": {
						"application": "my-service",
						"@timestamp": "2026-01-01T00:00:00Z"
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	fields, err := d.DetectServiceFields(context.Background(), "logs-*")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("expected at least one detected field")
	}
	found := false
	for _, f := range fields {
		if f == "application" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'application' in detected fields, got %v", fields)
	}
}

func TestDetectServiceFieldsFallsBackToDefaultsWhenNoHits(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	fields, err := d.DetectServiceFields(context.Background(), "logs-*")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("expected default fields when no hits found")
	}
}

func TestDetectServiceFieldsFallsBackToDefaultsWhenNoFieldsDetected(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Document has none of the candidate service fields.
		_, _ = w.Write([]byte(`{
			"hits": {
				"hits": [{
					"_id": "1",
					"_index": "logs-app",
					"_source": {
						"@timestamp": "2026-01-01T00:00:00Z",
						"msg": "hello"
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	fields, err := d.DetectServiceFields(context.Background(), "logs-*")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("expected default fallback fields")
	}
}

func TestDetectServiceFieldsReturnsErrorOnServerFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	d := newTestDiscovery(server.URL)
	_, err := d.DetectServiceFields(context.Background(), "logs-*")
	if err == nil {
		t.Fatal("expected error when sample search returns 500")
	}
}

func TestDiscoverIndicesSendsAuthorizationHeader(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{{"index": "logs-app"}})
	}))
	defer server.Close()

	d := NewIndexDiscovery(server.URL, "my-api-key", &http.Client{Timeout: 5 * time.Second})
	_, err := d.DiscoverIndices(context.Background(), "logs-*")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotAuth != "ApiKey my-api-key" {
		t.Fatalf("expected ApiKey header, got %q", gotAuth)
	}
}
