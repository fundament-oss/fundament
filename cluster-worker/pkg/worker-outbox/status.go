package worker_outbox

import (
	"context"
	"fmt"
	"time"
)

// StatusConfig holds configuration for the status polling loop.
type StatusConfig struct {
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"30s"`
}

// RunStatusLoop runs a simple ticker that calls all registered StatusHandlers periodically.
// This runs as a separate goroutine, keeping status polling independent from outbox processing.
func (w *OutboxWorker) RunStatusLoop(ctx context.Context, cfg StatusConfig) error {
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	w.logger.Info("status loop starting", "poll_interval", cfg.PollInterval)

	w.runAllStatusHandlers(ctx) // Initial poll on startup

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("status loop stopped: %w", ctx.Err())
		case <-ticker.C:
			w.runAllStatusHandlers(ctx)
		}
	}
}

func (w *OutboxWorker) runAllStatusHandlers(ctx context.Context) {
	for _, h := range w.registry.StatusHandlers() {
		if err := h.CheckStatus(ctx); err != nil {
			w.logger.Error("status handler failed", "error", err)
		}
	}
}
