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
	"cerebron/internal/usecase/analyzecausalhints"
	"cerebron/internal/usecase/analyzeincident"
	"cerebron/internal/usecase/detectincidenttrends"
	"cerebron/internal/usecase/findsimilarincidents"
	"cerebron/internal/usecase/getincidenthistory"
	"cerebron/internal/usecase/getrecentdeployments"
	"cerebron/internal/usecase/getservicedependencies"
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

type detectIncidentTrendsParams struct {
	Service string `json:"service,omitempty"`
}

type getServiceDependenciesParams struct {
	Service string `json:"service,omitempty"`
}

type analyzeCausalHintsParams struct {
	Analysis domain.IncidentAnalysis `json:"analysis"`
}

type getRecentDeploymentsParams struct {
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type getIncidentHistoryParams struct {
	Service string `json:"service,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// GetRecentDeploymentsUseCase is the contract expected by the get_recent_deployments MCP tool.
type GetRecentDeploymentsUseCase interface {
	Run(ctx context.Context, input getrecentdeployments.Input) (getrecentdeployments.Result, error)
}

// GetIncidentHistoryUseCase is the contract expected by the get_incident_history MCP tool.
type GetIncidentHistoryUseCase interface {
	Run(ctx context.Context, input getincidenthistory.Input) (getincidenthistory.Result, error)
}

// NewMCPHandler returns a gin.HandlerFunc that serves the MCP streamable HTTP protocol.
// It exposes analyze_incident, find_similar_incidents, detect_incident_trends, get_service_dependencies, analyze_causal_hints, get_recent_deployments, and get_incident_history tools.
func NewMCPHandler(usecase AnalyzeIncidentUseCase, similarUsecase FindSimilarIncidentsUseCase, trendsUsecase DetectIncidentTrendsUseCase, depsUsecase GetServiceDependenciesUseCase, causalUsecase AnalyzeCausalHintsUseCase, deploymentsUsecase GetRecentDeploymentsUseCase, historyUsecase GetIncidentHistoryUseCase, log *slog.Logger, m *metrics.Metrics) gin.HandlerFunc {
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

	sdkmcp.AddTool(server,
		&sdkmcp.Tool{Name: "detect_incident_trends", Description: "Detect incident frequency, severity trends, and service degradation patterns"},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, params detectIncidentTrendsParams) (*sdkmcp.CallToolResult, domain.IncidentTrends, error) {
			start := time.Now()
			result, err := trendsUsecase.Run(ctx, detectincidenttrends.Input{Service: params.Service})
			latency := time.Since(start)
			if err != nil {
				logger.Enrich(log, ctx).ErrorContext(ctx, "mcp tool failed",
					"tool", "detect_incident_trends",
					"latency_ms", latency.Milliseconds(),
					"error", err,
				)
				m.MCPRequestsTotal.WithLabelValues("detect_incident_trends", "error").Inc()
				m.MCPRequestsDuration.WithLabelValues("detect_incident_trends").Observe(latency.Seconds())
				return nil, domain.IncidentTrends{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "detect_incident_trends",
				"latency_ms", latency.Milliseconds(),
				"services", len(result.Services),
			)
			m.MCPRequestsTotal.WithLabelValues("detect_incident_trends", "ok").Inc()
			m.MCPRequestsDuration.WithLabelValues("detect_incident_trends").Observe(latency.Seconds())

			return nil, result, nil
		},
	)

	sdkmcp.AddTool(server,
		&sdkmcp.Tool{Name: "get_service_dependencies", Description: "Get upstream/downstream dependencies and blast radius for a service"},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, params getServiceDependenciesParams) (*sdkmcp.CallToolResult, domain.ServiceDependencies, error) {
			start := time.Now()
			result, err := depsUsecase.Run(ctx, getservicedependencies.Input{Service: params.Service})
			latency := time.Since(start)
			if err != nil {
				logger.Enrich(log, ctx).ErrorContext(ctx, "mcp tool failed",
					"tool", "get_service_dependencies",
					"latency_ms", latency.Milliseconds(),
					"error", err,
				)
				m.MCPRequestsTotal.WithLabelValues("get_service_dependencies", "error").Inc()
				m.MCPRequestsDuration.WithLabelValues("get_service_dependencies").Observe(latency.Seconds())
				return nil, domain.ServiceDependencies{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "get_service_dependencies",
				"latency_ms", latency.Milliseconds(),
				"upstreams", len(result.Upstreams),
				"downstreams", len(result.Downstreams),
				"blast_radius", len(result.BlastRadius),
			)
			m.MCPRequestsTotal.WithLabelValues("get_service_dependencies", "ok").Inc()
			m.MCPRequestsDuration.WithLabelValues("get_service_dependencies").Observe(latency.Seconds())

			return nil, result, nil
		},
	)

	sdkmcp.AddTool(server,
		&sdkmcp.Tool{Name: "analyze_causal_hints", Description: "Apply deterministic causal heuristics to an incident analysis (deployment triggers, DB latency, infra degradation)"},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, params analyzeCausalHintsParams) (*sdkmcp.CallToolResult, domain.CausalAnalysis, error) {
			start := time.Now()
			result, err := causalUsecase.Run(ctx, analyzecausalhints.Input{Analysis: params.Analysis})
			latency := time.Since(start)
			if err != nil {
				logger.Enrich(log, ctx).ErrorContext(ctx, "mcp tool failed",
					"tool", "analyze_causal_hints",
					"latency_ms", latency.Milliseconds(),
					"error", err,
				)
				m.MCPRequestsTotal.WithLabelValues("analyze_causal_hints", "error").Inc()
				m.MCPRequestsDuration.WithLabelValues("analyze_causal_hints").Observe(latency.Seconds())
				return nil, domain.CausalAnalysis{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "analyze_causal_hints",
				"latency_ms", latency.Milliseconds(),
				"hints", len(result.Hints),
			)
			m.MCPRequestsTotal.WithLabelValues("analyze_causal_hints", "ok").Inc()
			m.MCPRequestsDuration.WithLabelValues("analyze_causal_hints").Observe(latency.Seconds())

			return nil, result, nil
		},
	)

	sdkmcp.AddTool(server,
		&sdkmcp.Tool{Name: "get_recent_deployments", Description: "Get recent deployments for a service (required) across all deployment providers"},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, params getRecentDeploymentsParams) (*sdkmcp.CallToolResult, getrecentdeployments.Result, error) {
			start := time.Now()
			result, err := deploymentsUsecase.Run(ctx, getrecentdeployments.Input{
				Service:     params.Service,
				Environment: params.Environment,
				Limit:       params.Limit,
			})
			latency := time.Since(start)
			if err != nil {
				logger.Enrich(log, ctx).ErrorContext(ctx, "mcp tool failed",
					"tool", "get_recent_deployments",
					"latency_ms", latency.Milliseconds(),
					"error", err,
				)
				m.MCPRequestsTotal.WithLabelValues("get_recent_deployments", "error").Inc()
				m.MCPRequestsDuration.WithLabelValues("get_recent_deployments").Observe(latency.Seconds())
				return nil, getrecentdeployments.Result{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "get_recent_deployments",
				"latency_ms", latency.Milliseconds(),
				"deployments", len(result.Deployments),
			)
			m.MCPRequestsTotal.WithLabelValues("get_recent_deployments", "ok").Inc()
			m.MCPRequestsDuration.WithLabelValues("get_recent_deployments").Observe(latency.Seconds())

			return nil, result, nil
		},
	)

	sdkmcp.AddTool(server,
		&sdkmcp.Tool{Name: "get_incident_history", Description: "Get historical incidents for a service (required), newest first"},
		func(ctx context.Context, _ *sdkmcp.CallToolRequest, params getIncidentHistoryParams) (*sdkmcp.CallToolResult, getincidenthistory.Result, error) {
			start := time.Now()
			result, err := historyUsecase.Run(ctx, getincidenthistory.Input{
				Service: params.Service,
				Limit:   params.Limit,
			})
			latency := time.Since(start)
			if err != nil {
				logger.Enrich(log, ctx).ErrorContext(ctx, "mcp tool failed",
					"tool", "get_incident_history",
					"latency_ms", latency.Milliseconds(),
					"error", err,
				)
				m.MCPRequestsTotal.WithLabelValues("get_incident_history", "error").Inc()
				m.MCPRequestsDuration.WithLabelValues("get_incident_history").Observe(latency.Seconds())
				return nil, getincidenthistory.Result{}, err
			}

			logger.Enrich(log, ctx).InfoContext(ctx, "mcp tool completed",
				"tool", "get_incident_history",
				"latency_ms", latency.Milliseconds(),
				"incidents", len(result.Incidents),
				"total", result.Total,
			)
			m.MCPRequestsTotal.WithLabelValues("get_incident_history", "ok").Inc()
			m.MCPRequestsDuration.WithLabelValues("get_incident_history").Observe(latency.Seconds())

			return nil, result, nil
		},
	)

	mcpHTTPHandler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return server
	}, nil)

	return gin.WrapH(mcpHTTPHandler)
}
