// Package reconcile implements the periodic reconciliation loop.
// It discovers missing work and enqueues it into the outbox.
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

// Config holds configuration for the reconciliation loop.
type Config struct {
	Interval time.Duration `env:"INTERVAL" envDefault:"5m"`
}

// Worker periodically calls all registered ReconcileHandlers.
type Worker struct {
	registry *handler.Registry
	logger   *slog.Logger
	cfg      Config
	ready    atomic.Bool
}

func New(registry *handler.Registry, logger *slog.Logger, cfg Config) *Worker {
	return &Worker{
		registry: registry,
		logger:   logger.With("worker", "reconcile"),
		cfg:      cfg,
	}
}

// IsReady returns true after the first reconciliation has completed.
func (w *Worker) IsReady() bool {
	return w.ready.Load()
}

// Run starts the reconciliation loop. It should be run as a separate goroutine.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting reconcile loop", "interval", w.cfg.Interval)

	// Run immediately on startup, then on the ticker interval.
	w.reconcileAll(ctx)
	w.ready.Store(true)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("reconcile loop stopped: %w", ctx.Err())
		case <-ticker.C:
			w.reconcileAll(ctx)
		}
	}
}

// reconcileAll delegates reconciliation to each registered ReconcileHandler.
// Each handler owns its own re-enqueue and orphan-detection logic.
func (w *Worker) reconcileAll(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	w.logger.Info("starting reconciliation")

	for _, h := range w.registry.ReconcileHandlers() {
		if err := h.Reconcile(ctx); err != nil {
			w.logger.Error("reconcile handler failed", "error", err)
		}
	}

	w.logger.Info("reconciliation complete")
}
