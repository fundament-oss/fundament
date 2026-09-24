package organization

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/auth"
)

// OrganizationHeader is the header name for selecting the active organization.
const OrganizationHeader = auth.OrganizationHeader

func (s *Server) authInterceptor() connect.Interceptor {
	return auth.NewInterceptor(s.authenticate)
}

// authenticate validates the JWT and injects user/org info into ctx.
// Returns an enriched context or a Connect error.
func (s *Server) authenticate(ctx context.Context, procedure string, header http.Header) (context.Context, error) {
	if s.isPublicEndpoint(procedure) {
		s.logger.DebugContext(ctx, "skipping auth for public endpoint", "procedure", procedure)
		return ctx, nil
	}

	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	userID := claims.UserID()
	ctx = WithUserID(ctx, userID)

	if s.isUserScopedEndpoint(procedure) {
		s.logger.DebugContext(ctx, "skipping organization check for user-scoped endpoint",
			"procedure", procedure, "user_id", userID)
		return ctx, nil
	}

	organizationID, err := auth.OrganizationFromHeader(claims, header)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a connect error
	}

	ctx = WithOrganizationID(ctx, organizationID)
	s.logger.DebugContext(ctx, "request authenticated", "organization_id", organizationID, "user_id", userID)
	return ctx, nil
}

// isPublicEndpoint checks if an endpoint should skip authentication.
// Public endpoints are defined in a map and can be extended as needed.
func (s *Server) isPublicEndpoint(procedure string) bool {
	publicEndpoints := map[string]bool{
		"/fundament.organization.v1.OrganizationService/HealthCheck": true,
		// Add more public endpoints as needed
		"/organization.v1.PluginService/GetPluginDefinition": true,
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
		// PutPluginDefinition is intentionally NOT user-scoped: publishing a
		// definition (which sets a plugin's image and cluster RBAC) requires a
		// valid Fun-Organization header and membership in that organization, so
		// any authenticated user cannot seed a definition. Deeper admin-only
		// gating is tracked as a follow-up once a role concept exists.
	}
	return userScopedEndpoints[procedure]
}
