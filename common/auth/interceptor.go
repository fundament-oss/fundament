package auth

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
)

// Authenticator validates an inbound request and returns a context enriched with
// the caller's identity, or a connect error if authentication fails. procedure
// is the fully-qualified RPC procedure name, allowing implementations to exempt
// public or otherwise specially-scoped endpoints.
type Authenticator func(ctx context.Context, procedure string, header http.Header) (context.Context, error)

// interceptor authenticates inbound unary and server-streaming requests using an
// Authenticator. It is a no-op for outbound client calls.
type interceptor struct {
	authenticate Authenticator
}

// NewInterceptor returns a connect.Interceptor that authenticates inbound
// requests using the given Authenticator.
func NewInterceptor(authenticate Authenticator) connect.Interceptor {
	return &interceptor{authenticate: authenticate}
}

func (i *interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := i.authenticate(ctx, req.Spec().Procedure, req.Header())
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient is a no-op: this interceptor only authenticates inbound
// server requests, not outbound client calls made by this service.
func (i *interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := i.authenticate(ctx, conn.Spec().Procedure, conn.RequestHeader())
		if err != nil {
			return err
		}
		return next(ctx, conn)
	}
}
