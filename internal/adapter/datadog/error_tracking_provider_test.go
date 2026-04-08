package datadog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cerebron/internal/config"
	"cerebron/internal/domain"
)

func TestErrorTrackingProviderFetchAlertsMapsIssues(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/error-tracking/issues/search":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}
			if r.Header.Get("DD-API-KEY") != "api-key" {
				t.Fatalf("unexpected api key header")
			}
			if r.Header.Get("DD-APPLICATION-KEY") != "app-key" {
				t.Fatalf("unexpected application key header")
			}

			var request searchIssuesRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if request.Data.Type != "search_request" {
				t.Fatalf("expected search_request type, got %q", request.Data.Type)
			}
			if request.Data.Attributes.Query != "service:presence-api env:preprod" {
				t.Fatalf("unexpected query %q", request.Data.Attributes.Query)
			}
			if request.Data.Attributes.Track != "trace" {
				t.Fatalf("unexpected track %q", request.Data.Attributes.Track)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id": "issue-1",
						"type": "error_tracking_search_result"
					}
				],
				"included": [
					{
						"id": "issue-1",
						"type": "issue",
						"attributes": {
							"error_message": "illegal base64 data at input byte 0",
							"error_type": "base64.CorruptInputError",
							"file_path": "common/introspect/jwt_token_introspect.go",
							"function_name": "jwtTokenIntrospect",
							"first_seen": 1775213899243,
							"last_seen": 1775213954616,
							"platform": "BACKEND",
							"service": "presence-api",
							"state": "OPEN"
						},
						"relationships": {
							"team_owners": {
								"data": [
									{
										"id": "team-1",
										"type": "team"
									}
								]
							}
						}
					},
					{
						"id": "team-1",
						"type": "team",
						"attributes": {
							"handle": "identity",
							"name": "Identity"
						}
					}
				]
			}`))
		case "/api/v2/spans/events/search":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewErrorTrackingProvider(config.DatadogConfig{
		APIKey: "api-key",
		AppKey: "app-key",
		ErrorTracking: config.DatadogErrorTrackingConfig{
			BaseURL: server.URL,
			Query:   "service:presence-api env:preprod",
			Track:   "trace",
		},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1775210000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Source != domain.SourceDatadog {
		t.Fatalf("expected datadog source, got %s", events[0].Source)
	}
	if events[0].Service != "presence-api" {
		t.Fatalf("expected service presence-api, got %q", events[0].Service)
	}
	if events[0].Environment != "preprod" {
		t.Fatalf("expected environment preprod, got %q", events[0].Environment)
	}
	if events[0].Severity != domain.SeverityAlert {
		t.Fatalf("expected alert severity, got %s", events[0].Severity)
	}
	if events[0].Message != "illegal base64 data at input byte 0" {
		t.Fatalf("expected error message, got %q", events[0].Message)
	}
	if events[0].Error != "base64.CorruptInputError: illegal base64 data at input byte 0" {
		t.Fatalf("unexpected error field %q", events[0].Error)
	}
	if events[0].OwnerTeam != "identity" {
		t.Fatalf("expected owner team identity, got %q", events[0].OwnerTeam)
	}
	if !events[0].OccurredAt.Equal(time.UnixMilli(1775213954616).UTC()) {
		t.Fatalf("expected last seen timestamp, got %s", events[0].OccurredAt.Format(time.RFC3339Nano))
	}
}

func TestErrorTrackingProviderFetchAlertsEnrichesIssueWithSpanContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/error-tracking/issues/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id": "issue-1",
						"type": "error_tracking_search_result"
					}
				],
				"included": [
					{
						"id": "issue-1",
						"type": "issue",
						"attributes": {
							"error_message": "illegal base64 data at input byte 0",
							"error_type": "base64.CorruptInputError",
							"first_seen": 1775213899243,
							"last_seen": 1775213954616,
							"service": "presence-api",
							"state": "OPEN"
						}
					}
				]
			}`))
		case "/api/v2/spans/events/search":
			var request spanSearchRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode span request: %v", err)
			}
			if request.Data.Attributes.Filter.Query != `service:presence-api env:preprod @error.type:"base64.CorruptInputError"` {
				t.Fatalf("unexpected span query %q", request.Data.Attributes.Filter.Query)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id": "span-1",
						"attributes": {
							"timestamp": "2026-04-03T10:59:14.616Z",
							"resource_name": "PUT /api/presence/me/status",
							"attributes": {
								"http.status_code": "401",
								"http.route": "/api/presence/me/status",
								"error.message": "illegal base64 data at input byte 0",
								"error.type": "base64.CorruptInputError"
							}
						}
					}
				]
			}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewErrorTrackingProvider(config.DatadogConfig{
		APIKey: "api-key",
		AppKey: "app-key",
		ErrorTracking: config.DatadogErrorTrackingConfig{
			BaseURL: server.URL,
			Query:   "service:presence-api env:preprod",
			Track:   "trace",
		},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1775210000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].StatusCode != 401 {
		t.Fatalf("expected status code 401, got %d", events[0].StatusCode)
	}
	if events[0].Route != "/api/presence/me/status" {
		t.Fatalf("expected route /api/presence/me/status, got %q", events[0].Route)
	}
	if !events[0].OccurredAt.Equal(time.Date(2026, 4, 3, 10, 59, 14, 616000000, time.UTC)) {
		t.Fatalf("expected span timestamp, got %s", events[0].OccurredAt.Format(time.RFC3339Nano))
	}
}

func TestErrorTrackingProviderFetchAlertsUsesFallbackBaseURL(t *testing.T) {
	t.Parallel()

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer failingServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"included":[]}`))
	}))
	defer successServer.Close()

	provider := NewErrorTrackingProvider(config.DatadogConfig{
		APIKey: "api-key",
		AppKey: "app-key",
		ErrorTracking: config.DatadogErrorTrackingConfig{
			BaseURL: failingServer.URL,
			Query:   "env:preprod",
			Track:   "trace",
		},
	})
	provider.baseURLs = []string{failingServer.URL, successServer.URL}

	events, err := provider.FetchAlerts(context.Background(), time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}
}

func TestErrorTrackingProviderFetchAlertsUsesIssueRelationshipIdentifier(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "search-result-1",
					"type": "error_tracking_search_result",
					"relationships": {
						"issue": {
							"data": {
								"id": "issue-42",
								"type": "issue"
							}
						}
					}
				}
			],
			"included": [
				{
					"id": "issue-42",
					"type": "issue",
					"attributes": {
						"error_message": "illegal base64 data at input byte 0",
						"error_type": "base64.CorruptInputError",
						"last_seen": 1775213954616,
						"service": "presence-api",
						"state": "OPEN"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewErrorTrackingProvider(config.DatadogConfig{
		APIKey: "api-key",
		AppKey: "app-key",
		ErrorTracking: config.DatadogErrorTrackingConfig{
			BaseURL: server.URL,
			Query:   "service:presence-api env:preprod",
			Track:   "trace",
		},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1775210000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Fingerprint != "datadog-error-tracking:search-result-1" {
		t.Fatalf("expected fingerprint based on search result id, got %q", events[0].Fingerprint)
	}
}

func TestErrorTrackingProviderFetchAlertsAcceptsLegacyIncludedIssueType(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "issue-legacy",
					"type": "error_tracking_search_result"
				}
			],
			"included": [
				{
					"id": "issue-legacy",
					"type": "error_tracking_issue",
					"attributes": {
						"error_message": "illegal base64 data at input byte 0",
						"error_type": "base64.CorruptInputError",
						"last_seen": 1775213954616,
						"service": "presence-api",
						"state": "OPEN"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewErrorTrackingProvider(config.DatadogConfig{
		APIKey: "api-key",
		AppKey: "app-key",
		ErrorTracking: config.DatadogErrorTrackingConfig{
			BaseURL: server.URL,
			Query:   "service:presence-api env:preprod",
			Track:   "trace",
		},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1775210000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestErrorTrackingProviderFetchAlertsLoadsIssueDetailsWhenSearchResponseHasNoIncluded(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/error-tracking/issues/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id": "2a8c8714-fa98-11f0-913f-da7ad0900002",
						"type": "error_tracking_search_result",
						"attributes": {
							"total_count": 4
						},
						"relationships": {
							"issue": {
								"data": {
									"id": "2a8c8714-fa98-11f0-913f-da7ad0900002",
									"type": "issue"
								}
							}
						}
					}
				]
			}`))
		case "/api/v2/error-tracking/issues/2a8c8714-fa98-11f0-913f-da7ad0900002":
			if r.URL.Query().Get("include") != "team_owners" {
				t.Fatalf("expected include=team_owners, got %q", r.URL.Query().Get("include"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"id": "2a8c8714-fa98-11f0-913f-da7ad0900002",
					"type": "issue",
					"attributes": {
						"error_message": "illegal base64 data at input byte 0",
						"error_type": "base64.CorruptInputError",
						"last_seen": 1775213954616,
						"service": "presence-api",
						"state": "OPEN"
					},
					"relationships": {
						"team_owners": {
							"data": [
								{
									"id": "team-1",
									"type": "team"
								}
							]
						}
					}
				},
				"included": [
					{
						"id": "team-1",
						"type": "team",
						"attributes": {
							"handle": "identity"
						}
					}
				]
			}`))
		case "/api/v2/spans/events/search":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewErrorTrackingProvider(config.DatadogConfig{
		APIKey: "api-key",
		AppKey: "app-key",
		ErrorTracking: config.DatadogErrorTrackingConfig{
			BaseURL: server.URL,
			Query:   "service:presence-api env:preprod",
			Track:   "trace",
		},
	})

	events, err := provider.FetchAlerts(context.Background(), time.Unix(1775210000, 0).UTC())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].OwnerTeam != "identity" {
		t.Fatalf("expected owner team identity, got %q", events[0].OwnerTeam)
	}
}
