package organization

import (
	"context"

	"github.com/google/uuid"
)

type contextKeyTenantID struct{}
type contextKeyClaims struct{}

// WithTenantID stores tenant_id in context.
func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKeyTenantID{}, tenantID)
}

// TenantIDFromContext extracts tenant_id from context.
// Returns the tenant ID and true if found, or zero UUID and false if not found.
func TenantIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	tenantID, ok := ctx.Value(contextKeyTenantID{}).(uuid.UUID)
	return tenantID, ok
}

// WithClaims stores full claims in context for additional metadata like Groups.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, contextKeyClaims{}, claims)
}

// ClaimsFromContext extracts claims from context.
// Returns the claims and true if found, or nil and false if not found.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(contextKeyClaims{}).(*Claims)
	return claims, ok
}
