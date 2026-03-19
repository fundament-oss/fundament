// Package reconcile implements the periodic reconciliation loop.
// It discovers missing work and enqueues it into the outbox.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

// maxConsecutiveFailures is the number of consecutive failed ticks before the worker exits.
const maxConsecutiveFailures = 3

// Config holds configuration for the reconciliation loop.
type Config struct {
	Interval time.Duration `env:"INTERVAL" envDefault:"5m"`
}

// Worker periodically calls all registered ReconcileHandlers.
type Worker struct {
	registry         *handler.Registry
	logger           *slog.Logger
	cfg              Config
	ready            atomic.Bool
	consecutiveFails int
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
	err := w.reconcileAll(ctx)
	if err := w.trackFails(err); err != nil {
		return err
	}
	w.ready.Store(true)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("reconcile loop stopped: %w", ctx.Err())
		case <-ticker.C:
			err := w.reconcileAll(ctx)
			if err := w.trackFails(err); err != nil {
				return err
			}
		}
	}
}

// reconcileAll delegates reconciliation to each registered ReconcileHandler.
// Each handler owns its own re-enqueue and orphan-detection logic.
func (w *Worker) reconcileAll(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // graceful shutdown
	}

	w.logger.Info("starting reconciliation")

	var errs []error
	for _, h := range w.registry.ReconcileHandlers() {
		if err := h.Reconcile(ctx); err != nil {
			w.logger.Error("reconcile handler failed", "error", err)
			errs = append(errs, err)
		}
	}

	w.logger.Info("reconciliation complete")
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	return nil
}

// trackFails tracks consecutive handler failures and returns a fatal error after maxConsecutiveFailures.
func (w *Worker) trackFails(err error) error {
	if err == nil {
		w.consecutiveFails = 0
		return nil
	}
	w.consecutiveFails++
	if w.consecutiveFails >= maxConsecutiveFailures {
		return fmt.Errorf("reconcile worker fatal: %d consecutive failures, last: %w", w.consecutiveFails, err)
	}
	w.logger.Warn("reconcile tick failed, will retry", "consecutive_failures", w.consecutiveFails, "error", err)
	return nil
}
