package handlerhttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cerebron/internal/logger"
	"cerebron/internal/metrics"
)

func LoggingMiddleware(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = logger.GenerateID()
		}
		correlationID := c.GetHeader("X-Correlation-Id")
		if correlationID == "" {
			correlationID = requestID
		}

		ctx := logger.WithRequestID(c.Request.Context(), requestID)
		ctx = logger.WithCorrelationID(ctx, correlationID)
		c.Request = c.Request.WithContext(ctx)

		c.Header("X-Request-Id", requestID)

		start := time.Now()
		c.Next()

		fields := []any{
			"method", c.Request.Method,
			"route", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
			"request_id", requestID,
			"correlation_id", correlationID,
		}

		log.InfoContext(ctx, "http request", fields...)
	}
}

func MetricsMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}
		statusLabel := fmt.Sprintf("%d", c.Writer.Status())
		m.HTTPRequestsTotal.WithLabelValues(c.Request.Method, route, statusLabel).Inc()
		m.HTTPRequestsDuration.WithLabelValues(c.Request.Method, route).Observe(duration)
	}
}

func RecoveryMiddleware(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.ErrorContext(c.Request.Context(), "panic recovered",
					"error", r,
					"method", c.Request.Method,
					"route", c.FullPath(),
					"request_id", logger.RequestID(c.Request.Context()),
				)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
