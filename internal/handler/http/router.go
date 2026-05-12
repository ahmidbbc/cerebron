package handlerhttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

func NewRouter(healthHandler HealthHandler, incidentHandler IncidentHandler, mcpHandler gin.HandlerFunc, log *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	// RecoveryMiddleware must be registered before LoggingMiddleware so panics are
	// caught and converted to 500 before the log line fires.
	router.Use(RecoveryMiddleware(log))
	router.Use(LoggingMiddleware(log))
	router.HandleMethodNotAllowed = true
	healthHandler.Register(router)
	incidentHandler.Register(router)
	router.POST("/mcp", mcpHandler)

	return router
}
