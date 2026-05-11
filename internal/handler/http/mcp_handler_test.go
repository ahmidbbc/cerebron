package handlerhttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cerebron/internal/domain"
)

func newMCPRouter() *gin.Engine {
	return NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{}),
	)
}

func TestMCPHandlerMountsOnMCPPath(t *testing.T) {
	t.Parallel()

	router := newMCPRouter()

	// POST /mcp must be handled (not 404). SDK returns 400 on an invalid body.
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected /mcp to be handled, got 404")
	}
}

// 2.2 — POST /mcp with tools/call analyze_incident returns 200 and a result.
// The MCP Streamable HTTP protocol requires an initialize handshake before tools/call.
func TestMCPHandlerAnalyzeIncidentEndToEnd(t *testing.T) {
	t.Parallel()

	stub := analyzeIncidentUseCaseStub{
		result: domain.IncidentAnalysis{
			Service:      "catalog-api",
			ModelVersion: domain.IncidentAnalysisModelVersion,
			Summary:      "No incident signals detected for service catalog-api.",
			Confidence:   0.0,
		},
	}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewMCPHandler(stub),
	)

	// Step 1: initialize the MCP session.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	initReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	router.ServeHTTP(initRec, initReq)

	if initRec.Code != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d — body: %s", initRec.Code, initRec.Body.String())
	}

	sessionID := initRec.Header().Get("Mcp-Session-Id")

	// Step 2: call analyze_incident.
	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"analyze_incident","arguments":{"service":"catalog-api","time_range":"1h"}}}`
	callReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(callBody))
	callReq.Header.Set("Content-Type", "application/json")
	callReq.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		callReq.Header.Set("Mcp-Session-Id", sessionID)
	}
	callRec := httptest.NewRecorder()
	router.ServeHTTP(callRec, callReq)

	if callRec.Code != http.StatusOK {
		t.Fatalf("tools/call: expected 200, got %d — body: %s", callRec.Code, callRec.Body.String())
	}
	if !bytes.Contains(callRec.Body.Bytes(), []byte("catalog-api")) {
		t.Fatalf("expected catalog-api in tool result, got: %s", callRec.Body.String())
	}
}

// 2.3 — MCP tool function contains no business logic: output equals exactly what the usecase returned.
func TestMCPHandlerToolDelegatesEntirelyToUseCase(t *testing.T) {
	t.Parallel()

	expected := domain.IncidentAnalysis{
		Service:      "payments",
		ModelVersion: domain.IncidentAnalysisModelVersion,
		Summary:      "sentinel-summary",
		Confidence:   0.99,
	}
	router := NewRouter(
		NewHealthHandler(healthUseCaseStub{}),
		NewIncidentHandler(analyzeIncidentUseCaseStub{}),
		NewMCPHandler(analyzeIncidentUseCaseStub{result: expected}),
	)

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	initReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	router.ServeHTTP(initRec, initReq)
	sessionID := initRec.Header().Get("Mcp-Session-Id")

	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"analyze_incident","arguments":{"service":"payments","time_range":"1h"}}}`
	callReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(callBody))
	callReq.Header.Set("Content-Type", "application/json")
	callReq.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		callReq.Header.Set("Mcp-Session-Id", sessionID)
	}
	callRec := httptest.NewRecorder()
	router.ServeHTTP(callRec, callReq)

	if callRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", callRec.Code, callRec.Body.String())
	}
	body := callRec.Body.Bytes()
	for _, sentinel := range []string{"sentinel-summary", "payments", "0.99"} {
		if !bytes.Contains(body, []byte(sentinel)) {
			t.Errorf("expected %q in tool result, got: %s", sentinel, body)
		}
	}
}

// 2.4 — MCP handler exposes exactly one tool: analyze_incident.
func TestMCPHandlerExposesExactlyOneToolAnalyzeIncident(t *testing.T) {
	t.Parallel()

	router := newMCPRouter()

	// Initialize session first.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	initReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	router.ServeHTTP(initRec, initReq)
	sessionID := initRec.Header().Get("Mcp-Session-Id")

	// List tools.
	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	listReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(listBody))
	listReq.Header.Set("Content-Type", "application/json")
	listReq.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		listReq.Header.Set("Mcp-Session-Id", sessionID)
	}
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("tools/list: expected 200, got %d — body: %s", listRec.Code, listRec.Body.String())
	}
	body := listRec.Body.Bytes()
	if !bytes.Contains(body, []byte(`"analyze_incident"`)) {
		t.Errorf("expected analyze_incident in tools list, got: %s", body)
	}
	// Ensure no other tool names are present by checking the tools array has exactly one entry.
	// The SDK serializes tools as {"tools":[{"name":"..."},...]}
	first := bytes.Index(body, []byte(`"name"`))
	last := bytes.LastIndex(body, []byte(`"name"`))
	if first != last {
		t.Errorf("expected exactly one tool, but found multiple 'name' fields in: %s", body)
	}
}

// 2.1 — Streamable HTTP spec: GET /mcp must return 405 (only POST is valid).
func TestMCPHandlerGetReturns405(t *testing.T) {
	t.Parallel()

	router := newMCPRouter()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, "/mcp", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp: want 405, got %d", method, rec.Code)
		}
	}
}
