package proxy

import (
	"context"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

type userIDContextKey struct{}

// WithUserID stores user_id in context.
func WithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

// UserIDFromContext extracts user_id from context.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDContextKey{}).(uuid.UUID)
	return userID, ok
}

// WithSAToken stores a ServiceAccount bearer token in context.
// Uses the kube package's context key so the reverse proxy Director can read it.
func WithSAToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, kube.SATokenContextKey{}, token)
}

// SATokenFrom returns the ServiceAccount bearer token stored in ctx, or the
// zero string if none is set.
func SATokenFrom(ctx context.Context) string {
	tok, _ := ctx.Value(kube.SATokenContextKey{}).(string)
	return tok
}
