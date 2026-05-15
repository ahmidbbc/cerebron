package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cerebron/internal/adapter/argocd"
	"cerebron/internal/adapter/datadog"
	"cerebron/internal/adapter/elasticsearch"
	"cerebron/internal/adapter/gerrit"
	"cerebron/internal/adapter/github"
	"cerebron/internal/adapter/gitlab"
	"cerebron/internal/adapter/jenkins"
	"cerebron/internal/adapter/kubernetes"
	"cerebron/internal/config"
	"cerebron/internal/diagnostics"
	handlerhttp "cerebron/internal/handler/http"
	"cerebron/internal/logger"
	"cerebron/internal/metrics"
	"cerebron/internal/port/outbound"
	"cerebron/internal/storage"
	"cerebron/internal/usecase/analyzecausalhints"
	"cerebron/internal/usecase/analyzeincident"
	"cerebron/internal/usecase/detectincidenttrends"
	"cerebron/internal/usecase/findsimilarincidents"
	"cerebron/internal/usecase/getservicedependencies"
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
	deploymentProviders := buildDeploymentProviders(cfg)
	analyzeIncidentService := analyzeincident.NewService(signalProviders, log,
		analyzeincident.WithProviderTimeout(cfg.Environment.ProviderTimeout),
		analyzeincident.WithMetrics(recorder),
		analyzeincident.WithRepository(incidentRepo),
		analyzeincident.WithDeploymentProviders(deploymentProviders),
	)
	similarIncidentsService := findsimilarincidents.NewService(incidentRepo)
	trendsService := detectincidenttrends.NewService(incidentRepo)
	depsService := getservicedependencies.NewService(nil)
	causalService := analyzecausalhints.NewService()
	incidentHandler := handlerhttp.NewIncidentHandler(analyzeIncidentService)
	similarIncidentsHandler := handlerhttp.NewSimilarIncidentsHandler(similarIncidentsService)
	trendsHandler := handlerhttp.NewTrendsHandler(trendsService)
	serviceDepsHandler := handlerhttp.NewServiceDependenciesHandler(depsService)
	causalHintsHandler := handlerhttp.NewCausalHintsHandler(causalService)
	mcpHandler := handlerhttp.NewMCPHandler(analyzeIncidentService, similarIncidentsService, trendsService, depsService, causalService, log, m)
	router := handlerhttp.NewRouter(healthHandler, incidentHandler, similarIncidentsHandler, trendsHandler, serviceDepsHandler, causalHintsHandler, mcpHandler, log, m, reg)

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

func buildDeploymentProviders(cfg config.Config) []outbound.DeploymentProvider {
	var providers []outbound.DeploymentProvider
	if cfg.GitHub.Enabled {
		providers = append(providers, github.NewDeploymentConnector(cfg.GitHub))
	}
	if cfg.GitLab.Enabled {
		providers = append(providers, gitlab.NewDeploymentConnector(cfg.GitLab))
	}
	if cfg.Gerrit.Enabled {
		providers = append(providers, gerrit.NewDeploymentConnector(cfg.Gerrit))
	}
	if cfg.Jenkins.Enabled {
		providers = append(providers, jenkins.NewDeploymentConnector(cfg.Jenkins))
	}
	if cfg.ArgoCD.Enabled {
		providers = append(providers, argocd.NewDeploymentConnector(cfg.ArgoCD))
	}
	if cfg.Kubernetes.Enabled {
		providers = append(providers, kubernetes.NewDeploymentConnector(cfg.Kubernetes, cfg.Kubernetes.APIServerURL, cfg.Kubernetes.Token))
	}
	return providers
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
