package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/outbox-worker/pkg/worker"
)

type config struct {
	Database     psqldb.Config
	OpenFGA      authz.Config
	LogLevel     slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"5s"`
	BatchSize    int32         `env:"BATCH_SIZE" envDefault:"100"`
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

	logger.Info("starting outbox-worker",
		"log_level", cfg.LogLevel.String(),
		"poll_interval", cfg.PollInterval,
		"batch_size", cfg.BatchSize,
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
	)

	authzClient, err := authz.New(cfg.OpenFGA)
	if err != nil {
		return fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	logger.Debug("OpenFGA client connected")

	w := worker.New(pool, authzClient, logger, worker.Config{
		PollInterval: cfg.PollInterval,
		BatchSize:    cfg.BatchSize,
	})

	if err := w.Run(ctx); err != nil {
		return fmt.Errorf("worker failed: %w", err)
	}
	return nil
}
