package circuitbreaker

import (
	"context"
	"errors"

	"connectrpc.com/connect"
)

// interceptor implements connect.Interceptor, rejecting both unary and
// streaming requests when the circuit breaker is open.
type interceptor struct {
	breaker *Breaker
}

// NewInterceptor returns a Connect interceptor that rejects all
// requests when the circuit breaker is open.
func NewInterceptor(breaker *Breaker) connect.Interceptor {
	return &interceptor{breaker: breaker}
}

func (i *interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if i.breaker.IsOpen() {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("service temporarily unavailable"))
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient is a passthrough — the circuit breaker only runs
// server-side, so outbound streaming client calls are not blocked.
func (i *interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if i.breaker.IsOpen() {
			return connect.NewError(connect.CodeUnavailable, errors.New("service temporarily unavailable"))
		}
		return next(ctx, conn)
	}
}
