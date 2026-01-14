package organization

import (
	"context"

	"connectrpc.com/connect"
)

// AuthInterceptor is a Connect unary interceptor that validates JWT and injects context.
// It skips authentication for public endpoints defined in isPublicEndpoint.
func (s *OrganizationServer) AuthInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Skip auth for public endpoints
			if s.isPublicEndpoint(req.Spec().Procedure) {
				s.logger.DebugContext(ctx, "skipping auth for public endpoint",
					"procedure", req.Spec().Procedure,
				)
				return next(ctx, req)
			}

			// Extract and validate JWT from Authorization header or Cookie
			claims, err := s.authValidator.Validate(req.Header())
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}

			// Inject into context
			ctx = WithOrganizationID(ctx, claims.OrganizationID)
			ctx = WithUserID(ctx, claims.UserID)
			ctx = WithClaims(ctx, claims)

			s.logger.DebugContext(ctx, "request authenticated",
				"organization_id", claims.OrganizationID,
				"user_id", claims.UserID,
			)

			// Call next handler with enriched context
			return next(ctx, req)
		}
	}
}

// isPublicEndpoint checks if an endpoint should skip authentication.
// Public endpoints are defined in a map and can be extended as needed.
func (s *OrganizationServer) isPublicEndpoint(procedure string) bool {
	publicEndpoints := map[string]bool{
		"/fundament.organization.v1.OrganizationService/HealthCheck": true,
		// Add more public endpoints as needed
	}
	return publicEndpoints[procedure]
}
