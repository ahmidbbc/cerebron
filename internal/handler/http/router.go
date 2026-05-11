package handlerhttp

import "github.com/gin-gonic/gin"

func NewRouter(healthHandler HealthHandler, incidentHandler IncidentHandler, mcpHandler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.HandleMethodNotAllowed = true
	healthHandler.Register(router)
	incidentHandler.Register(router)
	router.POST("/mcp", mcpHandler)

	return router
}
