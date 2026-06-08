package organization

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// OrganizationHeader is the header name for selecting the active organization.
const OrganizationHeader = "Fun-Organization"

// authInterceptorImpl implements connect.Interceptor for both unary and server-streaming calls.
type authInterceptorImpl struct {
	s *Server
}

func (s *Server) authInterceptor() connect.Interceptor {
	return &authInterceptorImpl{s: s}
}

func (a *authInterceptorImpl) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := a.s.authenticate(ctx, req.Spec().Procedure, req.Header())
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (a *authInterceptorImpl) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (a *authInterceptorImpl) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := a.s.authenticate(ctx, conn.Spec().Procedure, conn.RequestHeader())
		if err != nil {
			return err
		}
		return next(ctx, conn)
	}
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

	orgHeader := header.Get(OrganizationHeader)
	if orgHeader == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing %s header", OrganizationHeader))
	}

	organizationID, err := uuid.Parse(orgHeader)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization ID: %w", err))
	}

	if !slices.Contains(claims.OrganizationIDs, organizationID) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("user is not a member of organization %s", organizationID))
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
