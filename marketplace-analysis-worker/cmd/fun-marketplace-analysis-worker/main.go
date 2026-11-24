// Command fun-marketplace-analysis-worker analyzes the container image of each
// submitted plugin version for the review backoffice (FUN-24).
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

	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-analysis-worker/pkg/analyze"
	"github.com/fundament-oss/fundament/marketplace-analysis-worker/pkg/worker"
)

type config struct {
	Database        psqldb.Config
	LogLevel        slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	HealthPort      int           `env:"HEALTH_PORT" envDefault:"8098"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`

	PollInterval  time.Duration `env:"POLL_INTERVAL" envDefault:"10s"`
	SweepInterval time.Duration `env:"SWEEP_INTERVAL" envDefault:"10m"`
	JobTimeout    time.Duration `env:"JOB_TIMEOUT" envDefault:"10m"`
	MaxRetries    int32         `env:"MAX_RETRIES" envDefault:"5"`

	WorkDir       string `env:"WORK_DIR" envDefault:"/work"`
	TrivyPath     string `env:"TRIVY_PATH" envDefault:"/trivy"`
	MaxImageBytes int64  `env:"MAX_IMAGE_BYTES" envDefault:"4294967296"`
	MaxFiles      int    `env:"MAX_FILES" envDefault:"200000"`
	// e.g. "localhost:5112=plugin-registry.localhost:5000"
	RegistryRewrites         map[string]string `env:"REGISTRY_REWRITES" envSeparator:"," envKeyValSeparator:"="`
	InsecureRegistries       []string          `env:"INSECURE_REGISTRIES" envSeparator:","`
	AllowedPrivateRegistries []string          `env:"ALLOWED_PRIVATE_REGISTRIES" envSeparator:","`
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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	logger.Info("starting marketplace-analysis-worker",
		"log_level", cfg.LogLevel.String(),
		"max_image_bytes", cfg.MaxImageBytes,
		"max_files", cfg.MaxFiles,
		"registry_rewrites", cfg.RegistryRewrites,
		"insecure_registries", cfg.InsecureRegistries,
		"allowed_private_registries", cfg.AllowedPrivateRegistries,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

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

	// TMPDIR points into the work volume in the chart, and starts out missing
	// on a fresh emptyDir.
	for _, dir := range []string{cfg.WorkDir, os.TempDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	analyzer := analyze.New(&analyze.Config{
		MaxImageBytes:            cfg.MaxImageBytes,
		MaxFiles:                 cfg.MaxFiles,
		RegistryRewrites:         cfg.RegistryRewrites,
		InsecureRegistries:       cfg.InsecureRegistries,
		AllowedPrivateRegistries: cfg.AllowedPrivateRegistries,
		TrivyPath:                cfg.TrivyPath,
		WorkDir:                  cfg.WorkDir,
	})

	w := worker.New(pool, analyzer, logger, worker.Config{
		PollInterval:  cfg.PollInterval,
		SweepInterval: cfg.SweepInterval,
		JobTimeout:    cfg.JobTimeout,
		MaxRetries:    cfg.MaxRetries,
	})

	healthServer := startHealthServer(cfg.HealthPort, logger, w)

	err = w.Run(ctx)

	logger.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
		logger.Error("health server shutdown error", "error", shutdownErr)
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("worker failed: %w", err)
	}
	logger.Info("marketplace-analysis-worker stopped")
	return nil
}

func startHealthServer(port int, logger *slog.Logger, w *worker.Worker) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(resp http.ResponseWriter, _ *http.Request) {
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(resp http.ResponseWriter, _ *http.Request) {
		if !w.IsReady() {
			resp.WriteHeader(http.StatusServiceUnavailable)
			_, _ = resp.Write([]byte("not ready"))
			return
		}
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write([]byte("ready"))
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Info("health server starting", "port", port)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server error", "error", err)
		}
	}()
	return server
}
