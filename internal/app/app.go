package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cerebron/internal/adapter/datadog"
	"cerebron/internal/adapter/elasticsearch"
	"cerebron/internal/config"
	"cerebron/internal/diagnostics"
	handlerhttp "cerebron/internal/handler/http"
	"cerebron/internal/logger"
	"cerebron/internal/metrics"
	"cerebron/internal/port/outbound"
	"cerebron/internal/storage"
	"cerebron/internal/usecase/analyzeincident"
	"cerebron/internal/usecase/findsimilarincidents"
	"cerebron/internal/usecase/health"
)

type App struct {
	config config.Config
	server *http.Server
	log    *slog.Logger
}

func New(cfg config.Config) *App {
	log := logger.New()
	diagnostics.RunStartupChecks(context.Background(), cfg, log)
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	recorder := metrics.NewRecorder(m)

	healthService := health.NewService()
	healthHandler := handlerhttp.NewHealthHandler(healthService)
	signalProviders := buildProviders(cfg)
	incidentRepo := storage.NewMemoryIncidentRepository()
	analyzeIncidentService := analyzeincident.NewService(signalProviders, log,
		analyzeincident.WithProviderTimeout(cfg.Environment.ProviderTimeout),
		analyzeincident.WithMetrics(recorder),
		analyzeincident.WithRepository(incidentRepo),
	)
	similarIncidentsService := findsimilarincidents.NewService(incidentRepo)
	incidentHandler := handlerhttp.NewIncidentHandler(analyzeIncidentService)
	similarIncidentsHandler := handlerhttp.NewSimilarIncidentsHandler(similarIncidentsService)
	mcpHandler := handlerhttp.NewMCPHandler(analyzeIncidentService, similarIncidentsService, log, m)
	router := handlerhttp.NewRouter(healthHandler, incidentHandler, similarIncidentsHandler, mcpHandler, log, m, reg)

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
