package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"cerebron/internal/adapter/datadog"
	"cerebron/internal/adapter/elasticsearch"
	"cerebron/internal/config"
	handlerhttp "cerebron/internal/handler/http"
	"cerebron/internal/logger"
	"cerebron/internal/port/outbound"
	"cerebron/internal/usecase/analyzeincident"
	"cerebron/internal/usecase/health"
)

type App struct {
	config config.Config
	server *http.Server
	log    *slog.Logger
}

func New(cfg config.Config) *App {
	log := logger.New()
	healthService := health.NewService()
	healthHandler := handlerhttp.NewHealthHandler(healthService)
	signalProviders := buildProviders(cfg)
	analyzeIncidentService := analyzeincident.NewService(signalProviders, log, analyzeincident.WithProviderTimeout(cfg.Environment.ProviderTimeout))
	incidentHandler := handlerhttp.NewIncidentHandler(analyzeIncidentService)
	mcpHandler := handlerhttp.NewMCPHandler(analyzeIncidentService, log)
	router := handlerhttp.NewRouter(healthHandler, incidentHandler, mcpHandler, log)

	return &App{
		config: cfg,
		log:    log,
		server: &http.Server{
			Addr:              cfg.HTTP.Address,
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func buildProviders(cfg config.Config) []outbound.SignalProvider {
	var signalProviders []outbound.SignalProvider

	if cfg.Datadog.Enabled {
		alertProvider := datadog.NewAlertProvider(cfg.Datadog)
		eventAlertProvider := datadog.NewEventAlertProvider(cfg.Datadog)
		signalProviders = append(signalProviders, alertProvider, eventAlertProvider)
	}
	if cfg.Datadog.ErrorTracking.Enabled {
		errorTrackingProvider := datadog.NewErrorTrackingProvider(cfg.Datadog)
		signalProviders = append(signalProviders, errorTrackingProvider)
	}
	if cfg.Elastic.Enabled {
		logProvider := elasticsearch.NewLogProvider(cfg.Elastic)
		signalProviders = append(signalProviders, logProvider)
	}

	return signalProviders
}

func (a *App) Run(ctx context.Context) error {
	a.log.InfoContext(ctx, "server starting", "addr", a.config.HTTP.Address)
	errCh := make(chan error, 1)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		a.log.InfoContext(ctx, "server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.HTTP.ShutdownTimeout)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		a.log.ErrorContext(ctx, "server failed", "error", err)
		return err
	}
}
