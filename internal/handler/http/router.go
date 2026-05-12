package handlerhttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cerebron/internal/metrics"
)

func NewRouter(healthHandler HealthHandler, incidentHandler IncidentHandler, mcpHandler gin.HandlerFunc, log *slog.Logger, m *metrics.Metrics, gatherer prometheus.Gatherer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	// RecoveryMiddleware must be registered before LoggingMiddleware so panics are
	// caught and converted to 500 before the log line fires.
	router.Use(RecoveryMiddleware(log))
	router.Use(LoggingMiddleware(log))
	router.Use(MetricsMiddleware(m))
	router.HandleMethodNotAllowed = true
	healthHandler.Register(router)
	incidentHandler.Register(router)
	router.POST("/mcp", mcpHandler)
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})))

	return router
}
