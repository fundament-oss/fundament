package auth

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
)

// NewAuthInterceptor returns a Connect unary interceptor that validates JWTs
// from the Authorization header or auth cookie and attaches claims to the context.
func NewAuthInterceptor(validator *Validator) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			tokenStr := extractTokenFromRequest(req)
			if tokenStr == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing authorization token"))
			}

			claims, err := validator.ValidateToken(tokenStr)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}

			ctx = WithClaims(ctx, claims)
			return next(ctx, req)
		}
	}
}

func extractTokenFromRequest(req connect.AnyRequest) string {
	authHeader := req.Header().Get("Authorization")
	if len(authHeader) >= 8 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}

	cookieHeader := req.Header().Get("Cookie")
	if cookieHeader == "" {
		return ""
	}

	for part := range strings.SplitSeq(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, AuthCookieName+"="); ok {
			return after
		}
	}

	return ""
}
