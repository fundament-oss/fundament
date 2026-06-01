package dcim

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
)

// authInterceptorImpl implements connect.Interceptor for both unary and server-streaming calls.
type authInterceptorImpl struct {
	s *Server
}

func (s *Server) authInterceptor() connect.Interceptor {
	return &authInterceptorImpl{s: s}
}

func (a *authInterceptorImpl) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := a.s.authenticate(ctx, req.Header())
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient is a no-op: this interceptor only authenticates inbound
// server requests, not outbound client calls made by this service.
func (a *authInterceptorImpl) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (a *authInterceptorImpl) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := a.s.authenticate(ctx, conn.RequestHeader())
		if err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// authenticate validates the DCIM JWT and injects the user ID into ctx.
func (s *Server) authenticate(ctx context.Context, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	userID := claims.UserID()
	ctx = WithUserID(ctx, userID)
	s.logger.DebugContext(ctx, "request authenticated", "user_id", userID)
	return ctx, nil
}
