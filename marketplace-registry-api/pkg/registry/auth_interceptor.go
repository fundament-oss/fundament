package registry

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/auth"
)

// OrganizationHeader selects the active organization, as on every other
// organization-scoped surface (FUN-6).
const OrganizationHeader = auth.OrganizationHeader

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

	organizationID, err := auth.OrganizationFromHeader(claims, header)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a connect error
	}

	return WithOrganizationID(ctx, organizationID), nil
}
