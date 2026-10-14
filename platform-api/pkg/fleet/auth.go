package fleet

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
)

type contextKeyUserID struct{}

// UserIDFromContext returns the staff member the request was authenticated as.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(contextKeyUserID{}).(uuid.UUID)
	return userID, ok
}

func (s *Server) authInterceptor() connect.Interceptor {
	return auth.NewInterceptor(s.authenticate)
}

// authenticate accepts only the DCIM token (dcim.users, the staff store), so a
// customer's console token is refused by issuer. The admin-host restriction
// stays as the outer boundary (FUN-20). No RPC here is organization-scoped.
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return context.WithValue(ctx, contextKeyUserID{}, claims.UserID()), nil
}
