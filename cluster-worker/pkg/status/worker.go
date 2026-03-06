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
	cfg      Config
}

func New(registry *handler.Registry, logger *slog.Logger, cfg Config) *Worker {
	return &Worker{
		registry: registry,
		logger:   logger.With("worker", "status"),
		cfg:      cfg,
	}
}

// Run starts the status polling loop. It should be run as a separate goroutine.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting status loop", "interval", w.cfg.Interval)

	// Run immediately on startup, then on the ticker interval.
	w.runAllHandlers(ctx)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("status loop stopped: %w", ctx.Err())
		case <-ticker.C:
			w.runAllHandlers(ctx)
		}
	}
}

// runAllHandlers delegates status checking to each registered StatusHandler.
func (w *Worker) runAllHandlers(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	for _, h := range w.registry.StatusHandlers() {
		if ctx.Err() != nil {
			return
		}
		if err := h.CheckStatus(ctx); err != nil {
			w.logger.Error("status handler failed", "error", err)
		}
	}
}
