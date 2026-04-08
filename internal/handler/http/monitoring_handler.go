package handlerhttp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cerebron/internal/usecase/monitor"
)

type MonitoringUseCase interface {
	Run(ctx context.Context, input monitor.Input) (monitor.Result, error)
}

type MonitoringHandler struct {
	usecase MonitoringUseCase
}

type monitoringRunRequest struct {
	Since        string   `json:"since"`
	Environments []string `json:"environments"`
	Debug        bool     `json:"debug"`
}

type monitoringRunResponse struct {
	AlertEvents          int                               `json:"alert_events"`
	LogEvents            int                               `json:"log_events"`
	CollectedEvents      int                               `json:"collected_events"`
	CorrelatedEvents     int                               `json:"correlated_events"`
	ObservedEnvironments []string                          `json:"observed_environments"`
	Reportable           bool                              `json:"reportable"`
	Score                int                               `json:"score"`
	Debug                []monitoringProviderDebugResponse `json:"debug,omitempty"`
	DebugEvents          []monitoringDebugEventResponse    `json:"debug_events,omitempty"`
	DebugCorrelations    []monitoringDebugCorrelation      `json:"debug_correlations,omitempty"`
}

type monitoringProviderDebugResponse struct {
	Provider              string   `json:"provider"`
	MonitorsFetched       int      `json:"monitors_fetched"`
	CandidateEvents       int      `json:"candidate_events"`
	ReturnedByProvider    int      `json:"returned_by_provider"`
	FilteredByEnvironment int      `json:"filtered_by_environment"`
	FilteredByTimeWindow  int      `json:"filtered_by_time_window"`
	IgnoredByStatus       int      `json:"ignored_by_status"`
	IgnoredBeforeSince    int      `json:"ignored_before_since"`
	SampleMonitorNames    []string `json:"sample_monitor_names,omitempty"`
	SampleTags            []string `json:"sample_tags,omitempty"`
}

type monitoringDebugEventResponse struct {
	Source      string `json:"source"`
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
	OccurredAt  string `json:"occurred_at"`
	StatusCode  int    `json:"status_code,omitempty"`
	Route       string `json:"route,omitempty"`
	Message     string `json:"message,omitempty"`
	Error       string `json:"error,omitempty"`
}

type monitoringDebugCorrelation struct {
	Alert   monitoringDebugEventResponse `json:"alert"`
	Log     monitoringDebugEventResponse `json:"log"`
	Score   int                          `json:"score"`
	Reasons []string                     `json:"reasons,omitempty"`
}

func NewMonitoringHandler(usecase MonitoringUseCase) MonitoringHandler {
	return MonitoringHandler{usecase: usecase}
}

func (h MonitoringHandler) Register(router gin.IRouter) {
	router.POST("/api/v1/monitoring/signals/run", h.handleRun)
}

func (h MonitoringHandler) handleRun(c *gin.Context) {
	var request monitoringRunRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	input := monitor.Input{
		Environments: request.Environments,
		Debug:        request.Debug,
	}

	if request.Since != "" {
		lookback, err := time.ParseDuration(request.Since)
		if err != nil || lookback <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "since must be a positive duration"})
			return
		}

		input.Since = time.Now().Add(-lookback)
	}

	result, err := h.usecase.Run(c.Request.Context(), input)
	if err != nil {
		response := gin.H{"message": "monitoring execution failed"}
		if request.Debug {
			response["details"] = err.Error()
		}
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	response := monitoringRunResponse{
		AlertEvents:          result.AlertEvents,
		LogEvents:            result.LogEvents,
		CollectedEvents:      result.CollectedEvents,
		CorrelatedEvents:     result.CorrelatedEvents,
		ObservedEnvironments: result.ObservedEnvironments,
		Reportable:           result.Reportable,
		Score:                result.Score,
	}
	if request.Debug {
		response.Debug = make([]monitoringProviderDebugResponse, 0, len(result.Debug))
		for _, providerDebug := range result.Debug {
			response.Debug = append(response.Debug, monitoringProviderDebugResponse{
				Provider:              providerDebug.Provider,
				MonitorsFetched:       providerDebug.MonitorsFetched,
				CandidateEvents:       providerDebug.CandidateEvents,
				ReturnedByProvider:    providerDebug.ReturnedByProvider,
				FilteredByEnvironment: providerDebug.FilteredByEnvironment,
				FilteredByTimeWindow:  providerDebug.FilteredByTimeWindow,
				IgnoredByStatus:       providerDebug.IgnoredByStatus,
				IgnoredBeforeSince:    providerDebug.IgnoredBeforeSince,
				SampleMonitorNames:    providerDebug.SampleMonitorNames,
				SampleTags:            providerDebug.SampleTags,
			})
		}
		response.DebugEvents = make([]monitoringDebugEventResponse, 0, len(result.DebugEvents))
		for _, event := range result.DebugEvents {
			response.DebugEvents = append(response.DebugEvents, monitoringDebugEventResponse{
				Source:      event.Source,
				Service:     event.Service,
				Environment: event.Environment,
				OccurredAt:  event.OccurredAt.UTC().Format(time.RFC3339),
				StatusCode:  event.StatusCode,
				Route:       event.Route,
				Message:     event.Message,
				Error:       event.Error,
			})
		}
		response.DebugCorrelations = make([]monitoringDebugCorrelation, 0, len(result.DebugCorrelations))
		for _, correlation := range result.DebugCorrelations {
			response.DebugCorrelations = append(response.DebugCorrelations, monitoringDebugCorrelation{
				Alert: monitoringDebugEventResponse{
					Source:      correlation.Alert.Source,
					Service:     correlation.Alert.Service,
					Environment: correlation.Alert.Environment,
					OccurredAt:  correlation.Alert.OccurredAt.UTC().Format(time.RFC3339),
					StatusCode:  correlation.Alert.StatusCode,
					Route:       correlation.Alert.Route,
					Message:     correlation.Alert.Message,
					Error:       correlation.Alert.Error,
				},
				Log: monitoringDebugEventResponse{
					Source:      correlation.Log.Source,
					Service:     correlation.Log.Service,
					Environment: correlation.Log.Environment,
					OccurredAt:  correlation.Log.OccurredAt.UTC().Format(time.RFC3339),
					StatusCode:  correlation.Log.StatusCode,
					Route:       correlation.Log.Route,
					Message:     correlation.Log.Message,
					Error:       correlation.Log.Error,
				},
				Score:   correlation.Score,
				Reasons: correlation.Reasons,
			})
		}
	}

	c.JSON(http.StatusOK, response)
}
