package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/app"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type config struct {
	DatabaseURL string     `env:"DATABASE_URL,required,notEmpty"`
	LogLevel    slog.Level `env:"LOG_LEVEL" envDefault:"info"`

	App app.Config `envPrefix:""`
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

	db, err := psqldb.New(ctx, logger, psqldb.Config{URL: cfg.DatabaseURL})
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer db.Close()

	application, err := app.New(db.Pool, logger, &cfg.App)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}

	if err := application.Run(ctx); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}
