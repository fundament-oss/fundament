package organization

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// OrganizationHeader is the header name for selecting the active organization.
const OrganizationHeader = "Fun-Organization"

// authInterceptor is a Connect unary interceptor that validates JWT and injects context.
// It skips authentication for public endpoints defined in isPublicEndpoint.
func (s *Server) authInterceptor() connect.UnaryInterceptorFunc {
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

			// Inject user info into context
			ctx = WithUserID(ctx, claims.UserID)
			ctx = WithClaims(ctx, claims)

			// Skip organization header check for user-scoped endpoints
			if s.isUserScopedEndpoint(req.Spec().Procedure) {
				s.logger.DebugContext(ctx, "skipping organization check for user-scoped endpoint",
					"procedure", req.Spec().Procedure,
					"user_id", claims.UserID,
				)
				return next(ctx, req)
			}

			// Extract organization ID from header
			orgHeader := req.Header().Get(OrganizationHeader)
			if orgHeader == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing %s header", OrganizationHeader))
			}

			organizationID, err := uuid.Parse(orgHeader)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization ID: %w", err))
			}

			// Validate user belongs to the organization
			if !slices.Contains(claims.OrganizationIDs, organizationID) {
				return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("user is not a member of organization %s", organizationID))
			}

			// Inject organization into context
			ctx = WithOrganizationID(ctx, organizationID)

			s.logger.DebugContext(ctx, "request authenticated",
				"organization_id", organizationID,
				"user_id", claims.UserID,
			)

			// Call next handler with enriched context
			return next(ctx, req)
		}
	}
}

// isPublicEndpoint checks if an endpoint should skip authentication.
// Public endpoints are defined in a map and can be extended as needed.
func (s *Server) isPublicEndpoint(procedure string) bool {
	publicEndpoints := map[string]bool{
		"/fundament.organization.v1.OrganizationService/HealthCheck": true,
		// Add more public endpoints as needed
	}
	return publicEndpoints[procedure]
}

// isUserScopedEndpoint checks if an endpoint is user-scoped and should skip organization header check.
// These endpoints operate on the user's data across all their organizations.
func (s *Server) isUserScopedEndpoint(procedure string) bool {
	userScopedEndpoints := map[string]bool{
		"/organization.v1.OrganizationService/ListOrganizations": true,
		"/organization.v1.InviteService/ListInvitations":         true,
		"/organization.v1.InviteService/AcceptInvitation":        true,
		"/organization.v1.InviteService/DeclineInvitation":       true,
	}
	return userScopedEndpoints[procedure]
}
