package cluster

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// Handler manages cluster lifecycle in Gardener (sync, status, orphan cleanup).
// TODO: move actual logic here from worker-sync and worker-status.
type Handler struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger.With("handler", "cluster")}
}

func (h *Handler) Sync(ctx context.Context, id uuid.UUID) error {
	h.logger.Info("cluster sync stub: no-op", "cluster_id", id)
	return nil
}

func (h *Handler) CheckStatus(ctx context.Context) error {
	h.logger.Info("cluster status stub: no-op")
	return nil
}

func (h *Handler) Reconcile(ctx context.Context) error {
	h.logger.Info("cluster reconcile stub: no-op")
	return nil
}
