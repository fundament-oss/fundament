package worker_status

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

// StatusWorker periodically polls all registered status handlers.
type StatusWorker struct {
	registry *handler.Registry
	logger   *slog.Logger
	cfg      Config
}

// Config holds configuration for the status poller.
type Config struct {
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"30s"`
}

func New(registry *handler.Registry, logger *slog.Logger, cfg Config) *StatusWorker {
	return &StatusWorker{
		registry: registry,
		logger:   logger.With("worker", "status"),
		cfg:      cfg,
	}
}

func (w *StatusWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	w.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("status poller stopped: %w", ctx.Err())
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *StatusWorker) poll(ctx context.Context) {
	for _, h := range w.registry.StatusHandlers() {
		if err := h.CheckStatus(ctx); err != nil {
			w.logger.Error("status check failed", "error", err)
		}
	}
}
