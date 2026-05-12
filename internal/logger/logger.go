package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
)

type contextKey int

const (
	requestIDKey contextKey = iota
	correlationIDKey
)

func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(os.Getenv("LOG_LEVEL")),
	}))
}

// WithRequestID stores a request ID in ctx.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// WithCorrelationID stores a correlation ID in ctx.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// RequestID returns the request ID from ctx, or "".
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// CorrelationID returns the correlation ID from ctx, or "".
func CorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(correlationIDKey).(string)
	return v
}

// Enrich returns log with request_id and correlation_id fields added from ctx when present.
func Enrich(log *slog.Logger, ctx context.Context) *slog.Logger {
	if id := RequestID(ctx); id != "" {
		log = log.With("request_id", id)
	}
	if id := CorrelationID(ctx); id != "" {
		log = log.With("correlation_id", id)
	}
	return log
}

// GenerateID returns a random 16-char hex string suitable for request/correlation IDs.
func GenerateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
