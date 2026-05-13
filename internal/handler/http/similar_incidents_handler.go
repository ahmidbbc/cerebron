package handlerhttp

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cerebron/internal/usecase/findsimilarincidents"
)

// FindSimilarIncidentsUseCase is the contract expected by SimilarIncidentsHandler.
type FindSimilarIncidentsUseCase interface {
	Run(ctx context.Context, input findsimilarincidents.Input) (findsimilarincidents.Result, error)
}

// SimilarIncidentsHandler handles similar incident search HTTP requests.
type SimilarIncidentsHandler struct {
	usecase FindSimilarIncidentsUseCase
}

func NewSimilarIncidentsHandler(usecase FindSimilarIncidentsUseCase) SimilarIncidentsHandler {
	return SimilarIncidentsHandler{usecase: usecase}
}

func (h SimilarIncidentsHandler) Register(router gin.IRouter) {
	router.GET("/api/v1/incidents/similar", h.handleFindSimilar)
}

func (h SimilarIncidentsHandler) handleFindSimilar(c *gin.Context) {
	fingerprint := c.Query("fingerprint")
	service := c.Query("service")

	if fingerprint == "" && service == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "fingerprint or service query parameter is required"})
		return
	}

	limit := 10
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "limit must be a positive integer"})
			return
		}
		limit = n
	}

	result, err := h.usecase.Run(c.Request.Context(), findsimilarincidents.Input{
		Fingerprint: fingerprint,
		Service:     service,
		Limit:       limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "similar incident search failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}
