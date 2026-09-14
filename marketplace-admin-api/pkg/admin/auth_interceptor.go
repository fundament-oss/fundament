package admin

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/auth"
)

func (s *Server) authInterceptor() connect.Interceptor {
	return auth.NewInterceptor(s.authenticate)
}

// authenticate resolves the reviewer. Unlike the registry there is no
// organization header: a reviewer acts on submissions from every publishing
// organization, so no RPC here is organization-scoped. The credential is the
// DCIM token (dcim.users, the staff store), so a customer's console token is
// refused by issuer; the admin-host restriction stays as the outer boundary
// (FUN-20).
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return WithUserID(ctx, claims.UserID()), nil
}
