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
	"cerebron/internal/usecase/analyzeincident"
)

type analyzeIncidentParams struct {
	Services  []string `json:"services,omitempty"`
	Service   string   `json:"service,omitempty"`
	TimeRange string   `json:"time_range,omitempty"`
}

// NewMCPHandler returns a gin.HandlerFunc that serves the MCP streamable HTTP protocol.
// It exposes the analyze_incident tool backed by the given usecase.
func NewMCPHandler(usecase AnalyzeIncidentUseCase, log *slog.Logger) gin.HandlerFunc {
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
				return nil, domain.IncidentAnalysis{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "analyze_incident",
				"latency_ms", latency.Milliseconds(),
				"confidence", result.Confidence,
				"groups", len(result.Groups),
			)

			return nil, result, nil
		},
	)

	mcpHTTPHandler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return server
	}, nil)

	return gin.WrapH(mcpHTTPHandler)
}
