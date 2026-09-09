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
// organization, so no RPC here is organization-scoped. "Is a Fundament
// reviewer" is the deployment boundary — this deployable is restricted to the
// admin host (FUN-20); which store reviewers authenticate against is still
// undecided, so the credential is the console UserToken meanwhile.
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return WithUserID(ctx, claims.UserID()), nil
}
