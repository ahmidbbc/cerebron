package handlerhttp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/analyzeincident"
)

// AnalyzeIncidentUseCase is the contract expected by IncidentHandler.
type AnalyzeIncidentUseCase interface {
	Run(ctx context.Context, input analyzeincident.Input) (domain.IncidentAnalysis, error)
}

// IncidentHandler handles incident analysis HTTP requests.
type IncidentHandler struct {
	usecase AnalyzeIncidentUseCase
}

type analyzeIncidentRequest struct {
	Services  []string `json:"services"`
	Service   string   `json:"service"`
	TimeRange string   `json:"time_range"`
}

// NewIncidentHandler returns a new IncidentHandler.
func NewIncidentHandler(usecase AnalyzeIncidentUseCase) IncidentHandler {
	return IncidentHandler{usecase: usecase}
}

// Register mounts the handler routes on the given router.
func (h IncidentHandler) Register(router gin.IRouter) {
	router.POST("/api/v1/incidents/analyze", h.handleAnalyze)
}

func (h IncidentHandler) handleAnalyze(c *gin.Context) {
	var request analyzeIncidentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	services := request.Services
	if len(services) == 0 && request.Service != "" {
		services = []string{request.Service}
	}
	input := analyzeincident.Input{
		Services: services,
	}

	if request.TimeRange != "" {
		lookback, err := time.ParseDuration(request.TimeRange)
		if err != nil || lookback <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "time_range must be a positive duration"})
			return
		}
		input.Since = time.Now().Add(-lookback)
	}

	result, err := h.usecase.Run(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "incident analysis failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}
