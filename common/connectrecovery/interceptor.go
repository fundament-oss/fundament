package connectrecovery

import (
	"context"
	"log/slog"
	"runtime/debug"

	"connectrpc.com/connect"
)

// NewInterceptor returns a connect interceptor that recovers from panics,
// logs them with a stack trace, and returns a generic internal error.
func NewInterceptor(logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					logger.ErrorContext(ctx, "panic recovered in handler",
						"panic", r,
						"stack", string(stack),
						"procedure", req.Spec().Procedure,
					)
					err = connect.NewError(connect.CodeInternal, nil)
				}
			}()
			return next(ctx, req)
		}
	}
}
