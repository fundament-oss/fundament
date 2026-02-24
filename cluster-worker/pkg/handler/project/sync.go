package project

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// Handler is a stub sync handler for projects.
// It logs the event and returns nil (success).
type Handler struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger.With("handler", "project")}
}

func (h *Handler) Sync(ctx context.Context, id uuid.UUID) error {
	h.logger.Info("project sync stub: no-op", "project_id", id)
	return nil
}
