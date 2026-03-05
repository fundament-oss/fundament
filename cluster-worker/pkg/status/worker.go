// Package status implements the periodic status polling loop.
package status

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

// Config holds configuration for the status polling loop.
type Config struct {
	Interval time.Duration `env:"INTERVAL" envDefault:"30s"`
}

// Worker periodically calls all registered StatusHandlers.
type Worker struct {
	registry *handler.Registry
	logger   *slog.Logger
}

func New(registry *handler.Registry, logger *slog.Logger) *Worker {
	return &Worker{
		registry: registry,
		logger:   logger.With("worker", "status"),
	}
}

// Run starts the status polling loop. It should be run as a separate goroutine.
func (w *Worker) Run(ctx context.Context, cfg Config) error {
	w.logger.Info("starting status loop", "interval", cfg.Interval)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("status loop stopped: %w", ctx.Err())
		case <-ticker.C:
			for _, h := range w.registry.StatusHandlers() {
				if ctx.Err() != nil {
					return fmt.Errorf("status loop stopped: %w", ctx.Err())
				}
				if err := h.CheckStatus(ctx); err != nil {
					w.logger.Error("status handler failed", "error", err)
				}
			}
		}
	}
}
