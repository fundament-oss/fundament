package namespace

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// Handler is a stub sync handler for namespaces.
// It logs the event and returns nil (success).
type Handler struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger.With("handler", "namespace")}
}

func (h *Handler) Sync(ctx context.Context, id uuid.UUID) error {
	h.logger.Info("namespace sync stub: no-op", "namespace_id", id)
	return nil
}
