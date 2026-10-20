package dcim

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/auth"
)

func (s *Server) authInterceptor() connect.Interceptor {
	return auth.NewInterceptor(s.authenticate)
}

// authenticate validates the DCIM JWT and injects the user ID into ctx.
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	userID := claims.UserID()
	ctx = auth.WithUserID(ctx, userID)
	s.logger.DebugContext(ctx, "request authenticated", "user_id", userID)
	return ctx, nil
}
