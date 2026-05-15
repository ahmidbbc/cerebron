package handlerhttp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"cerebron/internal/domain"
	"cerebron/internal/usecase/getservicedependencies"
)

// GetServiceDependenciesUseCase is the contract expected by ServiceDependenciesHandler.
type GetServiceDependenciesUseCase interface {
	Run(ctx context.Context, input getservicedependencies.Input) (domain.ServiceDependencies, error)
}

// ServiceDependenciesHandler handles service dependency graph HTTP requests.
type ServiceDependenciesHandler struct {
	usecase GetServiceDependenciesUseCase
}

func NewServiceDependenciesHandler(usecase GetServiceDependenciesUseCase) ServiceDependenciesHandler {
	return ServiceDependenciesHandler{usecase: usecase}
}

func (h ServiceDependenciesHandler) Register(router gin.IRouter) {
	router.GET("/api/v1/services/dependencies", h.handleDependencies)
}

func (h ServiceDependenciesHandler) handleDependencies(c *gin.Context) {
	service := c.Query("service")

	deps, err := h.usecase.Run(c.Request.Context(), getservicedependencies.Input{Service: service})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "dependency graph unavailable"})
		return
	}

	c.JSON(http.StatusOK, deps)
}
