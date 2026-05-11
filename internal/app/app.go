package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"cerebron/internal/adapter/datadog"
	"cerebron/internal/adapter/elasticsearch"
	"cerebron/internal/config"
	handlerhttp "cerebron/internal/handler/http"
	"cerebron/internal/port/outbound"
	"cerebron/internal/usecase/analyzeincident"
	"cerebron/internal/usecase/health"
)

type App struct {
	config config.Config
	server *http.Server
}

func New(cfg config.Config) *App {
	healthService := health.NewService()
	healthHandler := handlerhttp.NewHealthHandler(healthService)
	signalProviders := buildProviders(cfg)
	analyzeIncidentService := analyzeincident.NewService(signalProviders)
	incidentHandler := handlerhttp.NewIncidentHandler(analyzeIncidentService)
	mcpHandler := handlerhttp.NewMCPHandler(analyzeIncidentService)
	router := handlerhttp.NewRouter(healthHandler, incidentHandler, mcpHandler)

	return &App{
		config: cfg,
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
	errCh := make(chan error, 1)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.HTTP.ShutdownTimeout)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
