package registry

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
)

// OrganizationHeader selects the active organization, as on every other
// organization-scoped surface (FUN-6).
const OrganizationHeader = "Fun-Organization"

func (s *Server) authInterceptor() connect.Interceptor {
	return auth.NewInterceptor(s.authenticate)
}

// authenticate resolves the caller and the organization they are acting for.
// Every registry.v1 RPC is organization-scoped — including the two that take an
// empty request — so there are no public or user-scoped exemptions here.
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	ctx = WithUserID(ctx, claims.UserID())

	orgHeader := header.Get(OrganizationHeader)
	if orgHeader == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("missing %s header", OrganizationHeader))
	}

	organizationID, err := uuid.Parse(orgHeader)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization ID: %w", err))
	}

	// The JWT carries the authoritative membership list, so this is settled
	// without a database round trip.
	if !slices.Contains(claims.OrganizationIDs, organizationID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user is not a member of organization %s", organizationID))
	}

	return WithOrganizationID(ctx, organizationID), nil
}
