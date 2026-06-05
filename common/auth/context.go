package auth

import (
	"context"

	"github.com/google/uuid"
)

type contextKeyUserID struct{}

// WithUserID stores user_id in context.
func WithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKeyUserID{}, userID)
}

// UserIDFromContext extracts user_id from context.
// Returns the user ID and true if found, or zero UUID and false if not found.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(contextKeyUserID{}).(uuid.UUID)
	return userID, ok
}
