package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"golang.org/x/sync/errgroup"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	clusterhandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/outbox"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/reconcile"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/status"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type config struct {
	DatabaseURL        string        `env:"DATABASE_URL,required,notEmpty"`
	GardenerMode       string        `env:"GARDENER_MODE"`       // mock or real
	GardenerKubeconfig string        `env:"GARDENER_KUBECONFIG"` // Required for real mode
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	HealthPort         int           `env:"HEALTH_PORT" envDefault:"8097"`
	ShutdownTimeout    time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`

	// Worker config
	Outbox    outbox.Config         `envPrefix:"OUTBOX_"`
	Status    status.Config         `envPrefix:"STATUS_"`
	Reconcile reconcile.Config      `envPrefix:"RECONCILE_"`
	Cluster   clusterhandler.Config `envPrefix:"CLUSTER_"`

	// Provider configuration for real Gardener mode.
	// These configure how Shoots are created in Gardener and depend on the target infrastructure.

	// ProviderType is the infrastructure provider (e.g., "local", "metal", "aws", "gcp", "azure").
	// Determines which Gardener extension is used to provision the cluster infrastructure.
	ProviderType string `env:"GARDENER_PROVIDER_TYPE"`

	// ProviderCloudProfile references a Gardener CloudProfile that defines available machine types,
	// images, and regions for the provider. Must exist in Gardener before creating Shoots.
	ProviderCloudProfile string `env:"GARDENER_CLOUD_PROFILE"`

	// ProviderCredentialsBindingName references a Gardener CredentialsBinding that contains
	// cloud provider credentials (e.g., AWS access keys, GCP service account).
	// Not needed for local provider. Required for real cloud providers.
	ProviderCredentialsBindingName string `env:"GARDENER_CREDENTIALS_BINDING_NAME"`

	// ProviderMachineImageName is the OS image for worker nodes (e.g., "local", "gardenlinux").
	ProviderMachineImageName string `env:"GARDENER_MACHINE_IMAGE_NAME"`

	// ProviderMachineImageVersion is the version of the OS image (e.g., "1.0.0", "1592.2.0").
	ProviderMachineImageVersion string `env:"GARDENER_MACHINE_IMAGE_VERSION"`

	// ProviderDefaultMachineType is the fallback machine type when a cluster has no node pools
	// (e.g., "local", "n1-standard-4").
	ProviderDefaultMachineType string `env:"GARDENER_DEFAULT_MACHINE_TYPE"`
}

// ReadyChecker reports whether a worker is ready to serve traffic.
type ReadyChecker interface {
	IsReady() bool
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("parse env: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Database
	db, err := psqldb.New(ctx, logger, psqldb.Config{URL: cfg.DatabaseURL})
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer db.Close()

	// Gardener client (mock or real)
	gardenerClient, err := createGardenerClient(&cfg, logger)
	if err != nil {
		return err
	}

	// Handler registry
	registry := handler.NewRegistry()

	// Cluster handler (sync, status, reconcile)
	ch := clusterhandler.New(db.Pool, gardenerClient, logger, cfg.Cluster)
	registry.RegisterSync(handler.EntityCluster, ch)
	registry.RegisterStatus(ch)
	registry.RegisterReconcile(ch)

	// Outbox worker
	outboxWorker := outbox.New(db.Pool, registry, logger, cfg.Outbox)

	// Status worker
	statusWorker := status.New(registry, logger, cfg.Status)

	// Reconcile worker
	reconcileWorker := reconcile.New(registry, logger, cfg.Reconcile)

	// Health check server
	healthServer := startHealthServer(&cfg, logger, outboxWorker, statusWorker, reconcileWorker)

	logger.Info("cluster-worker starting",
		"poll_interval", cfg.Outbox.PollInterval,
		"reconcile_interval", cfg.Reconcile.Interval,
		"status_interval", cfg.Status.Interval,
		"max_retries", cfg.Outbox.MaxRetries,
		"gardener_mode", cfg.GardenerMode)

	// Run outbox worker and status loop concurrently
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return outboxWorker.Run(ctx)
	})

	g.Go(func() error {
		return statusWorker.Run(ctx)
	})

	g.Go(func() error {
		return reconcileWorker.Run(ctx)
	})

	err = g.Wait()

	// Graceful shutdown
	logger.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
		logger.Error("health server shutdown error", "error", shutdownErr)
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("worker error: %w", err)
	}

	logger.Info("cluster-worker stopped")
	return nil
}

func createGardenerClient(cfg *config, logger *slog.Logger) (gardener.Client, error) {
	switch cfg.GardenerMode {
	case "mock":
		logger.Info("using mock Gardener client (in-memory)")
		return gardener.NewMock(logger), nil

	case "real":
		if cfg.GardenerKubeconfig == "" {
			return nil, fmt.Errorf("GARDENER_KUBECONFIG required for real mode")
		}
		// Start with defaults for local provider, override with env values
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

func startHealthServer(cfg *config, logger *slog.Logger, checkers ...ReadyChecker) *http.Server {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(resp http.ResponseWriter, _ *http.Request) {
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
		Addr:              fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("health server starting", "port", cfg.HealthPort)
		if err := healthServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server error", "error", err)
		}
	}()

	return healthServer
}
