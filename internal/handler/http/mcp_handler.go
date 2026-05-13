package handlerhttp

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gin-gonic/gin"

	"cerebron/internal/domain"
	"cerebron/internal/logger"
	"cerebron/internal/metrics"
	"cerebron/internal/usecase/analyzeincident"
	"cerebron/internal/usecase/findsimilarincidents"
)

type analyzeIncidentParams struct {
	Services  []string `json:"services,omitempty"`
	Service   string   `json:"service,omitempty"`
	TimeRange string   `json:"time_range,omitempty"`
}

type findSimilarIncidentsParams struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	Service     string `json:"service,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

// NewMCPHandler returns a gin.HandlerFunc that serves the MCP streamable HTTP protocol.
// It exposes the analyze_incident and find_similar_incidents tools.
func NewMCPHandler(usecase AnalyzeIncidentUseCase, similarUsecase FindSimilarIncidentsUseCase, log *slog.Logger, m *metrics.Metrics) gin.HandlerFunc {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "cerebron",
		Version: "1.0",
	}, nil)

	sdkmcp.AddTool(server,
		&sdkmcp.Tool{Name: "analyze_incident", Description: "Analyze incident signals for a service"},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, params analyzeIncidentParams) (*sdkmcp.CallToolResult, domain.IncidentAnalysis, error) {
			start := time.Now()
			services := params.Services
			if len(services) == 0 && params.Service != "" {
				services = []string{params.Service}
			}
			input := analyzeincident.Input{Services: services}
			if params.TimeRange != "" {
				if lookback, err := time.ParseDuration(params.TimeRange); err == nil && lookback > 0 {
					input.Since = time.Now().Add(-lookback)
				}
			}

			result, err := usecase.Run(ctx, input)
			latency := time.Since(start)
			if err != nil {
				logger.Enrich(log, ctx).ErrorContext(ctx, "mcp tool failed",
					"tool", "analyze_incident",
					"latency_ms", latency.Milliseconds(),
					"error", err,
				)
				m.MCPRequestsTotal.WithLabelValues("analyze_incident", "error").Inc()
				m.MCPRequestsDuration.WithLabelValues("analyze_incident").Observe(latency.Seconds())
				return nil, domain.IncidentAnalysis{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "analyze_incident",
				"latency_ms", latency.Milliseconds(),
				"confidence", result.Confidence,
				"groups", len(result.Groups),
			)
			m.MCPRequestsTotal.WithLabelValues("analyze_incident", "ok").Inc()
			m.MCPRequestsDuration.WithLabelValues("analyze_incident").Observe(latency.Seconds())

			return nil, result, nil
		},
	)

	sdkmcp.AddTool(server,
		&sdkmcp.Tool{Name: "find_similar_incidents", Description: "Find historical incidents similar to a given fingerprint or service"},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, params findSimilarIncidentsParams) (*sdkmcp.CallToolResult, findsimilarincidents.Result, error) {
			start := time.Now()
			result, err := similarUsecase.Run(ctx, findsimilarincidents.Input{
				Fingerprint: params.Fingerprint,
				Service:     params.Service,
				Limit:       params.Limit,
			})
			latency := time.Since(start)
			if err != nil {
				logger.Enrich(log, ctx).ErrorContext(ctx, "mcp tool failed",
					"tool", "find_similar_incidents",
					"latency_ms", latency.Milliseconds(),
					"error", err,
				)
				m.MCPRequestsTotal.WithLabelValues("find_similar_incidents", "error").Inc()
				m.MCPRequestsDuration.WithLabelValues("find_similar_incidents").Observe(latency.Seconds())
				return nil, findsimilarincidents.Result{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "find_similar_incidents",
				"latency_ms", latency.Milliseconds(),
				"related_count", len(result.Related),
			)
			m.MCPRequestsTotal.WithLabelValues("find_similar_incidents", "ok").Inc()
			m.MCPRequestsDuration.WithLabelValues("find_similar_incidents").Observe(latency.Seconds())

			return nil, result, nil
		},
	)

	mcpHTTPHandler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return server
	}, nil)

	return gin.WrapH(mcpHTTPHandler)
}
