package handlerhttp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/analyzecausalhints"
)

// AnalyzeCausalHintsUseCase is the contract expected by CausalHintsHandler.
type AnalyzeCausalHintsUseCase interface {
	Run(ctx context.Context, input analyzecausalhints.Input) (domain.CausalAnalysis, error)
}

// CausalHintsHandler handles causal hint analysis HTTP requests.
type CausalHintsHandler struct {
	usecase AnalyzeCausalHintsUseCase
}

func NewCausalHintsHandler(usecase AnalyzeCausalHintsUseCase) CausalHintsHandler {
	return CausalHintsHandler{usecase: usecase}
}

func (h CausalHintsHandler) Register(router gin.IRouter) {
	router.POST("/api/v1/incidents/causal-hints", h.handleCausalHints)
}

func (h CausalHintsHandler) handleCausalHints(c *gin.Context) {
	var analysis domain.IncidentAnalysis
	if err := c.ShouldBindJSON(&analysis); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	result, err := h.usecase.Run(c.Request.Context(), analyzecausalhints.Input{Analysis: analysis})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "causal hint analysis failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}
