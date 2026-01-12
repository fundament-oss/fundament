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

	"github.com/fundament-oss/fundament/cluster-worker/pkg/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/worker"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type config struct {
	DatabaseURL         string        `env:"DATABASE_URL,required,notEmpty"`
	GardenerMode        string        `env:"GARDENER_MODE" envDefault:"mock"` // mock or real
	GardenerKubeconfig  string        `env:"GARDENER_KUBECONFIG"`             // Required for real mode
	GardenerNamespace   string        `env:"GARDENER_NAMESPACE" envDefault:"garden-fundament"`
	LogLevel            slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	PollInterval        time.Duration `env:"POLL_INTERVAL" envDefault:"30s"`
	ReconcileInterval   time.Duration `env:"RECONCILE_INTERVAL" envDefault:"5m"`
	StatusPollInterval  time.Duration `env:"STATUS_POLL_INTERVAL" envDefault:"30s"`
	StatusPollBatchSize int32         `env:"STATUS_POLL_BATCH_SIZE" envDefault:"50"`
	HealthPort          int           `env:"HEALTH_PORT" envDefault:"8097"`
	ShutdownTimeout     time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`

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

	// ProviderMaxShootNameLen limits Shoot names due to infrastructure constraints.
	// Local provider requires 21 chars max (node name length limits).
	// Cloud providers typically allow up to 63 chars.
	ProviderMaxShootNameLen int `env:"GARDENER_MAX_SHOOT_NAME_LEN"`
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

	// SyncWorker (syncs manifests to Gardener)
	w := worker.NewSyncWorker(db.Pool, gardenerClient, logger, worker.Config{
		PollInterval:      cfg.PollInterval,
		ReconcileInterval: cfg.ReconcileInterval,
	})

	// StatusWorker (monitors Gardener reconciliation)
	sp := worker.NewStatusWorker(db.Pool, gardenerClient, logger, worker.StatusConfig{
		PollInterval: cfg.StatusPollInterval,
		BatchSize:    cfg.StatusPollBatchSize,
	})

	// Health check server
	healthServer := startHealthServer(&cfg, w, logger)

	logger.Info("cluster-worker starting",
		"poll_interval", cfg.PollInterval,
		"reconcile_interval", cfg.ReconcileInterval,
		"status_poll_interval", cfg.StatusPollInterval,
		"gardener_mode", cfg.GardenerMode,
		"gardener_namespace", cfg.GardenerNamespace)

	// Run both worker and status poller concurrently
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return w.Run(ctx)
	})

	g.Go(func() error {
		return sp.Run(ctx)
	})

	err = g.Wait()

	// Graceful shutdown
	logger.Info("shutting down...")
	w.Shutdown(cfg.ShutdownTimeout)
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
		if cfg.ProviderMaxShootNameLen > 0 {
			providerCfg.MaxShootNameLen = cfg.ProviderMaxShootNameLen
		}

		logger.Info("using real Gardener client",
			"kubeconfig", cfg.GardenerKubeconfig,
			"namespace", cfg.GardenerNamespace,
			"provider", providerCfg.Type,
			"cloudProfile", providerCfg.CloudProfile)
		client, err := gardener.NewReal(cfg.GardenerKubeconfig, cfg.GardenerNamespace, providerCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("create gardener client: %w", err)
		}
		return client, nil

	default:
		return nil, fmt.Errorf("invalid GARDENER_MODE: %s (must be mock or real)", cfg.GardenerMode)
	}
}

func startHealthServer(cfg *config, w *worker.SyncWorker, logger *slog.Logger) *http.Server {
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
