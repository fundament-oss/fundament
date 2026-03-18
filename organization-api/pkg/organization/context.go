package organization

import (
	"context"

	"github.com/google/uuid"
)

type contextKeyOrganizationID struct{}
type contextKeyUserID struct{}

// WithOrganizationID stores organization_id in context.
func WithOrganizationID(ctx context.Context, organizationID uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKeyOrganizationID{}, organizationID)
}

// OrganizationIDFromContext extracts organization_id from context.
// Returns the organization ID and true if found, or zero UUID and false if not found.
func OrganizationIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	organizationID, ok := ctx.Value(contextKeyOrganizationID{}).(uuid.UUID)
	return organizationID, ok
}

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
