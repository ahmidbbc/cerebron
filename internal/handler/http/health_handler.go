package handlerhttp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type HealthUseCase interface {
	Liveness(ctx context.Context) error
	Readiness(ctx context.Context) error
}

type HealthHandler struct {
	usecase HealthUseCase
}

func NewHealthHandler(usecase HealthUseCase) HealthHandler {
	return HealthHandler{usecase: usecase}
}

func (h HealthHandler) Register(router gin.IRouter) {
	router.GET("/healthz", h.handleLiveness)
	router.GET("/readyz", h.handleReadiness)
}

func (h HealthHandler) handleLiveness(c *gin.Context) {
	if err := h.usecase.Liveness(c.Request.Context()); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	c.Status(http.StatusOK)
}

func (h HealthHandler) handleReadiness(c *gin.Context) {
	if err := h.usecase.Readiness(c.Request.Context()); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	c.Status(http.StatusOK)
}
