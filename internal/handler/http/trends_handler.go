package handlerhttp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/detectincidenttrends"
)

// DetectIncidentTrendsUseCase is the contract expected by TrendsHandler.
type DetectIncidentTrendsUseCase interface {
	Run(ctx context.Context, input detectincidenttrends.Input) (domain.IncidentTrends, error)
}

// TrendsHandler handles incident trend HTTP requests.
type TrendsHandler struct {
	usecase DetectIncidentTrendsUseCase
}

func NewTrendsHandler(usecase DetectIncidentTrendsUseCase) TrendsHandler {
	return TrendsHandler{usecase: usecase}
}

func (h TrendsHandler) Register(router gin.IRouter) {
	router.GET("/api/v1/incidents/trends", h.handleTrends)
}

func (h TrendsHandler) handleTrends(c *gin.Context) {
	service := c.Query("service")

	trends, err := h.usecase.Run(c.Request.Context(), detectincidenttrends.Input{Service: service})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "trend detection failed"})
		return
	}

	c.JSON(http.StatusOK, trends)
}
