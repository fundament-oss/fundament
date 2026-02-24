package projectmember

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// Handler is a stub sync handler for project members.
// It logs the event and returns nil (success).
type Handler struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger.With("handler", "projectmember")}
}

func (h *Handler) Sync(ctx context.Context, id uuid.UUID) error {
	h.logger.Info("project member sync stub: no-op", "project_member_id", id)
	return nil
}
