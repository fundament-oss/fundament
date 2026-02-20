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
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	clusterhandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	namespacehandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/namespace"
	projecthandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/project"
	projectmemberhandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/projectmember"
	worker_outbox "github.com/fundament-oss/fundament/cluster-worker/pkg/worker-outbox"
	worker_status "github.com/fundament-oss/fundament/cluster-worker/pkg/worker-status"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type config struct {
	DatabaseURL        string        `env:"DATABASE_URL,required,notEmpty"`
	GardenerMode       string        `env:"GARDENER_MODE"`       // mock or real
	GardenerKubeconfig string        `env:"GARDENER_KUBECONFIG"` // Required for real mode
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	HealthPort         int           `env:"HEALTH_PORT" envDefault:"8097"`
	ShutdownTimeout    time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`

	// Worker configs (env tags defined in worker package)
	Outbox worker_outbox.Config `envPrefix:"OUTBOX_"`
	Status worker_status.Config `envPrefix:"STATUS_"`

	// Provider configuration for real Gardener mode.
	ProviderType                   string `env:"GARDENER_PROVIDER_TYPE"`
	ProviderCloudProfile           string `env:"GARDENER_CLOUD_PROFILE"`
	ProviderCredentialsBindingName string `env:"GARDENER_CREDENTIALS_BINDING_NAME"`
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
	database, err := psqldb.New(ctx, logger, psqldb.Config{URL: cfg.DatabaseURL})
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer database.Close()

	// Gardener client (mock or real)
	gardenerClient, err := createGardenerClient(&cfg, logger)
	if err != nil {
		return err
	}

	queries := db.New(database.Pool)

	// Build handler registry
	registry := handler.NewRegistry()

	clusterHandler := clusterhandler.New(queries, gardenerClient, logger)
	registry.RegisterSync(handler.EntityCluster, clusterHandler)
	registry.RegisterStatus(clusterHandler)
	registry.RegisterReconcile(clusterHandler)

	registry.RegisterSync(handler.EntityNamespace, namespacehandler.New(logger))
	registry.RegisterSync(handler.EntityProjectMember, projectmemberhandler.New(logger))
	registry.RegisterSync(handler.EntityProject, projecthandler.New(logger))

	// Workers
	outboxWorker := worker_outbox.New(database.Pool, registry, logger, cfg.Outbox)
	statusWorker := worker_status.New(registry, logger, cfg.Status)

	// Health check server
	healthServer := startHealthServer(&cfg, outboxWorker, logger)

	logger.Info("cluster-worker starting",
		"poll_interval", cfg.Outbox.PollInterval,
		"reconcile_interval", cfg.Outbox.ReconcileInterval,
		"status_poll_interval", cfg.Status.PollInterval,
		"max_retries", cfg.Outbox.MaxRetries,
		"gardener_mode", cfg.GardenerMode)

	// Run both workers concurrently
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return outboxWorker.Run(ctx)
	})

	g.Go(func() error {
		return statusWorker.Run(ctx)
	})

	err = g.Wait()

	// Graceful shutdown
	logger.Info("shutting down...")
	if shutdownErr := healthServer.Shutdown(context.Background()); shutdownErr != nil {
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

func startHealthServer(cfg *config, w *worker_outbox.OutboxWorker, logger *slog.Logger) *http.Server {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(resp http.ResponseWriter, _ *http.Request) {
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write([]byte("ok"))
	})
	healthMux.HandleFunc("/readyz", func(resp http.ResponseWriter, _ *http.Request) {
		if w.IsReady() {
			resp.WriteHeader(http.StatusOK)
			_, _ = resp.Write([]byte("ready"))
		} else {
			resp.WriteHeader(http.StatusServiceUnavailable)
			_, _ = resp.Write([]byte("not ready"))
		}
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
