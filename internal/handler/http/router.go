package handlerhttp

import "github.com/gin-gonic/gin"

func NewRouter(healthHandler HealthHandler, monitoringHandler MonitoringHandler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	healthHandler.Register(router)
	monitoringHandler.Register(router)

	return router
}
