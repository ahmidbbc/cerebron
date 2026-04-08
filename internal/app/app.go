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
	"cerebron/internal/usecase/health"
	"cerebron/internal/usecase/monitor"
	reportingusecase "cerebron/internal/usecase/reporting"
)

type App struct {
	config config.Config
	server *http.Server
}

func New(cfg config.Config) *App {
	healthService := health.NewService()
	healthHandler := handlerhttp.NewHealthHandler(healthService)
	reportingService := reportingusecase.NewService(reportingusecase.DefaultPolicy())
	alertProviders := buildAlertProviders(cfg)
	logProviders := buildLogProviders(cfg)
	monitoringService := monitor.NewService(
		alertProviders,
		logProviders,
		reportingService,
		cfg.Environment.DefaultPollInterval,
		cfg.Environment.Mode,
		cfg.Environment.ObservedEnvs,
	)
	monitoringHandler := handlerhttp.NewMonitoringHandler(monitoringService)
	router := handlerhttp.NewRouter(healthHandler, monitoringHandler)

	return &App{
		config: cfg,
		server: &http.Server{
			Addr:              cfg.HTTP.Address,
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func buildAlertProviders(cfg config.Config) []outbound.AlertProvider {
	providers := make([]outbound.AlertProvider, 0)

	if cfg.Datadog.Enabled {
		providers = append(providers, datadog.NewAlertProvider(cfg.Datadog))
		providers = append(providers, datadog.NewEventAlertProvider(cfg.Datadog))
	}
	if cfg.Datadog.ErrorTracking.Enabled {
		providers = append(providers, datadog.NewErrorTrackingProvider(cfg.Datadog))
	}

	return providers
}

func buildLogProviders(cfg config.Config) []outbound.LogProvider {
	providers := make([]outbound.LogProvider, 0)

	if cfg.Elastic.Enabled {
		providers = append(providers, elasticsearch.NewLogProvider(cfg.Elastic))
	}

	return providers
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
