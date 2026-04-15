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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openfga/go-sdk/client"
	"golang.org/x/sync/errgroup"

	"github.com/fundament-oss/fundament/authz-worker/pkg/worker"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type config struct {
	Database        psqldb.Config
	OpenFGA         authz.Config
	LogLevel        slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	PollInterval    time.Duration `env:"POLL_INTERVAL" envDefault:"5s"`
	BatchSize       int32         `env:"BATCH_SIZE" envDefault:"100"`
	BaseBackoff     time.Duration `env:"BASE_BACKOFF" envDefault:"500ms"`
	MaxBackoff      time.Duration `env:"MAX_BACKOFF" envDefault:"5s"`
	MaxRetries      int32         `env:"MAX_RETRIES" envDefault:"3"`
	BackoffDelay    time.Duration `env:"BACKOFF_DELAY" envDefault:"5s"`
	HealthPort      int           `env:"HEALTH_PORT" envDefault:"8097"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
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
		return fmt.Errorf("env parse: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting authz-worker",
		"log_level", cfg.LogLevel.String(),
		"poll_interval", cfg.PollInterval,
		"batch_size", cfg.BatchSize,
		"base_backoff", cfg.BaseBackoff,
		"max_backoff", cfg.MaxBackoff,
		"max_retries", cfg.MaxRetries,
		"backoff_delay", cfg.BackoffDelay,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Debug("connecting to database")

	pgxcfg, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxcfg)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	dbversion.MustAssertLatestVersion(ctx, logger, pool)

	logger.Debug("database connected")

	logger.Debug("connecting to OpenFGA",
		"api_url", cfg.OpenFGA.APIURL,
		"store_id", cfg.OpenFGA.StoreID,
		"authorization_model_id", cfg.OpenFGA.AuthorizationModelID,
	)

	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:               cfg.OpenFGA.APIURL,
		StoreId:              cfg.OpenFGA.StoreID,
		AuthorizationModelId: cfg.OpenFGA.AuthorizationModelID,
	})
	if err != nil {
		return fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	// Validate the store and authorization model exist - this ensures we have valid config
	store, err := fgaClient.GetStore(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to validate OpenFGA store: %w", err)
	}

	model, err := fgaClient.ReadAuthorizationModel(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to validate OpenFGA authorization model: %w", err)
	}

	logger.Debug("OpenFGA client connected",
		"store_name", store.GetName(),
		"model_id", model.AuthorizationModel.GetId(),
	)

	w := worker.New(pool, fgaClient, logger, worker.Config{
		PollInterval: cfg.PollInterval,
		BatchSize:    cfg.BatchSize,
		BaseBackoff:  cfg.BaseBackoff,
		MaxBackoff:   cfg.MaxBackoff,
		MaxRetries:   cfg.MaxRetries,
		BackoffDelay: cfg.BackoffDelay,
	})

	healthServer := startHealthServer(&cfg, logger, w)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return w.Run(ctx)
	})

	err = g.Wait()

	// Graceful shutdown of the health server
	logger.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
		logger.Error("health server shutdown error", "error", shutdownErr)
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("worker failed: %w", err)
	}

	logger.Info("authz-worker stopped")
	return nil
}

func startHealthServer(cfg *config, logger *slog.Logger, checkers ...ReadyChecker) *http.Server {
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
