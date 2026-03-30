// Package app wires up the cluster-worker application.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	clusterhandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/usersync"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/outbox"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/reconcile"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/status"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Config holds all configuration for the cluster-worker application.
type Config struct {
	HealthPort      int           `env:"HEALTH_PORT" envDefault:"8097"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`

	GardenerMode       string `env:"GARDENER_MODE"`
	GardenerKubeconfig string `env:"GARDENER_KUBECONFIG"`

	// Provider configuration for real Gardener mode.
	ProviderType                   string `env:"GARDENER_PROVIDER_TYPE"`
	ProviderCloudProfile           string `env:"GARDENER_CLOUD_PROFILE"`
	ProviderCredentialsBindingName string `env:"GARDENER_CREDENTIALS_BINDING_NAME"`
	ProviderMachineImageName       string `env:"GARDENER_MACHINE_IMAGE_NAME"`
	ProviderMachineImageVersion    string `env:"GARDENER_MACHINE_IMAGE_VERSION"`
	ProviderDefaultMachineType     string `env:"GARDENER_DEFAULT_MACHINE_TYPE"`

	Outbox    outbox.Config         `envPrefix:"OUTBOX_"`
	Status    status.Config         `envPrefix:"STATUS_"`
	Reconcile reconcile.Config      `envPrefix:"RECONCILE_"`
	Cluster   clusterhandler.Config `envPrefix:"CLUSTER_"`
}

// ReadyChecker reports whether a worker is ready to serve traffic.
type ReadyChecker interface {
	IsReady() bool
}

// App holds the wired-up application components.
type App struct {
	pool            *pgxpool.Pool
	registry        *handler.Registry
	outboxWorker    *outbox.Worker
	statusWorker    *status.Worker
	reconcileWorker *reconcile.Worker
	healthServer    *http.Server
	logger          *slog.Logger
	cfg             *Config
}

// New creates and wires up the cluster-worker application.
func New(pool *pgxpool.Pool, logger *slog.Logger, cfg *Config) (*App, error) {
	gardenerClient, err := createGardenerClient(cfg, logger)
	if err != nil {
		return nil, err
	}

	registry := handler.NewRegistry()

	// Cluster handler (sync, status, reconcile)
	ch := clusterhandler.New(pool, gardenerClient, gardenerClient, logger, cfg.Cluster)
	registry.RegisterSync(handler.EntityCluster, ch)
	registry.RegisterSync(handler.EntityNodePool, ch)
	registry.RegisterStatus(ch)
	registry.RegisterReconcile(ch)

	// User sync handler (SA/CRB lifecycle on shoots)
	shootAccess, err := createShootAccess(cfg.GardenerMode, gardenerClient, logger)
	if err != nil {
		return nil, err
	}
	ush := usersync.New(pool, shootAccess, logger)
	registry.RegisterSync(handler.EntityOrgUser, ush)
	registry.RegisterSync(handler.EntityProjectMember, ush)
	registry.RegisterSyncForEvent(handler.EntityCluster, dbconst.ClusterOutboxEvent_Ready, ush)
	registry.RegisterReconcile(ush)

	// Workers
	outboxWorker := outbox.New(pool, registry, logger, cfg.Outbox)
	statusWorker := status.New(registry, logger, cfg.Status)
	reconcileWorker := reconcile.New(registry, logger, cfg.Reconcile)

	// Health server
	healthServer := startHealthServer(cfg.HealthPort, logger, outboxWorker, statusWorker, reconcileWorker)

	return &App{
		pool:            pool,
		registry:        registry,
		outboxWorker:    outboxWorker,
		statusWorker:    statusWorker,
		reconcileWorker: reconcileWorker,
		healthServer:    healthServer,
		logger:          logger,
		cfg:             cfg,
	}, nil
}

// Run starts all workers concurrently and blocks until shutdown.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("cluster-worker starting",
		"poll_interval", a.cfg.Outbox.PollInterval,
		"reconcile_interval", a.cfg.Reconcile.Interval,
		"status_interval", a.cfg.Status.Interval,
		"max_retries", a.cfg.Outbox.MaxRetries,
		"gardener_mode", a.cfg.GardenerMode)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return a.outboxWorker.Run(ctx) })
	g.Go(func() error { return a.statusWorker.Run(ctx) })
	g.Go(func() error { return a.reconcileWorker.Run(ctx) })

	err := g.Wait()

	// Graceful shutdown
	a.logger.Info("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()
	if shutdownErr := a.healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
		a.logger.Error("health server shutdown error", "error", shutdownErr)
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("worker error: %w", err)
	}

	a.logger.Info("cluster-worker stopped")
	return nil
}

func createGardenerClient(cfg *Config, logger *slog.Logger) (gardener.Client, error) {
	switch cfg.GardenerMode {
	case "mock":
		logger.Info("using mock Gardener client (in-memory)")
		return gardener.NewMock(logger), nil

	case "real":
		if cfg.GardenerKubeconfig == "" {
			return nil, fmt.Errorf("GARDENER_KUBECONFIG required for real mode")
		}
		providerCfg := gardener.NewProviderConfig()
		if cfg.ProviderType != "" {
			providerCfg.Type = cfg.ProviderType
		}
		if cfg.ProviderCloudProfile != "" {
			providerCfg.CloudProfile = cfg.ProviderCloudProfile
		}
		if cfg.ProviderCredentialsBindingName != "" {
			providerCfg.CredentialsBindingName = cfg.ProviderCredentialsBindingName
		}
		if cfg.ProviderMachineImageName != "" {
			providerCfg.MachineImageName = cfg.ProviderMachineImageName
		}
		if cfg.ProviderMachineImageVersion != "" {
			providerCfg.MachineImageVersion = cfg.ProviderMachineImageVersion
		}
		if cfg.ProviderDefaultMachineType != "" {
			providerCfg.DefaultMachineType = cfg.ProviderDefaultMachineType
		}

		logger.Info("using real Gardener client",
			"kubeconfig", cfg.GardenerKubeconfig,
			"provider", providerCfg.Type,
			"cloudProfile", providerCfg.CloudProfile)
		client, err := gardener.NewReal(cfg.GardenerKubeconfig, providerCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("create gardener client: %w", err)
		}
		return client, nil

	default:
		return nil, fmt.Errorf("invalid GARDENER_MODE: %s (must be mock or real)", cfg.GardenerMode)
	}
}

func createShootAccess(gardenerMode string, gardenerClient gardener.Client, logger *slog.Logger) (shoot.ShootAccess, error) {
	switch gardenerMode {
	case "mock":
		logger.Info("using mock shoot access (in-memory)")
		return shoot.NewMockShootAccess(logger), nil
	case "real":
		logger.Info("using real shoot access (AdminKubeconfigRequest)")
		return shoot.NewRealShootAccess(gardenerClient, logger), nil
	default:
		return nil, fmt.Errorf("invalid GARDENER_MODE: %s (must be mock or real)", gardenerMode)
	}
}

func startHealthServer(port int, logger *slog.Logger, checkers ...ReadyChecker) *http.Server {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/livez", func(resp http.ResponseWriter, _ *http.Request) {
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write([]byte("ok"))
	})
	healthMux.HandleFunc("/readyz", func(resp http.ResponseWriter, _ *http.Request) {
		for _, c := range checkers {
			if !c.IsReady() {
				resp.WriteHeader(http.StatusServiceUnavailable)
				_, _ = resp.Write([]byte("not ready"))
				return
			}
		}
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write([]byte("ready"))
	})

	healthServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           healthMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("health server starting", "port", port)
		if err := healthServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server error", "error", err)
		}
	}()

	return healthServer
}
