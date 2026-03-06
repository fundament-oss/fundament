// Package status implements the periodic status polling loop.
package status

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

// Config holds configuration for the status polling loop.
type Config struct {
	Interval time.Duration `env:"INTERVAL" envDefault:"30s"`
}

// Worker periodically calls all registered StatusHandlers.
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
		logger:   logger.With("worker", "status"),
		cfg:      cfg,
	}
}

// IsReady returns true after the first status poll has completed.
func (w *Worker) IsReady() bool {
	return w.ready.Load()
}

// Run starts the status polling loop. It should be run as a separate goroutine.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting status loop", "interval", w.cfg.Interval)

	// Run immediately on startup, then on the ticker interval.
	err := w.runAllHandlers(ctx)
	if err := w.trackFails(err); err != nil {
		return err
	}
	w.ready.Store(true)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("status loop stopped: %w", ctx.Err())
		case <-ticker.C:
			err := w.runAllHandlers(ctx)
			if err := w.trackFails(err); err != nil {
				return err
			}
		}
	}
}

// runAllHandlers delegates status checking to each registered StatusHandler.
func (w *Worker) runAllHandlers(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // gracefull shutdown
	}

	var errs []error
	for _, h := range w.registry.StatusHandlers() {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // gracefull shutdown
		}
		if err := h.CheckStatus(ctx); err != nil {
			w.logger.Error("status handler failed", "error", err)
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("status: %w", err)
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
		return fmt.Errorf("status worker fatal: %d consecutive failures, last: %w", w.consecutiveFails, err)
	}
	w.logger.Warn("status tick failed, will retry", "consecutive_failures", w.consecutiveFails, "error", err)
	return nil
}
