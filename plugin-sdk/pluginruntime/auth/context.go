package auth

import (
	"context"

	"github.com/google/uuid"
)

type contextKey int

const (
	claimsKey contextKey = iota
)

// WithClaims returns a new context with the given claims attached.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext extracts claims from the context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*Claims)
	return claims, ok
}

// UserIDFromContext extracts the user ID from claims in the context.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return claims.UserID, true
}

// OrganizationIDsFromContext extracts the organization IDs from claims in the context.
func OrganizationIDsFromContext(ctx context.Context) ([]uuid.UUID, bool) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return nil, false
	}
	return claims.OrganizationIDs, true
}
