package client

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

const (
	idempotencyHeaderKey    = "Idempotency-Key"
	idempotencyHeaderStatus = "Idempotency-Status"

	statusPending    = "pending"
	statusRetrying   = "retrying"
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusFailed     = "failed"

	idempotencyInitialBackoff = 100 * time.Millisecond
	idempotencyMaxBackoff     = 2 * time.Second
	idempotencyTotalBudget    = 30 * time.Second
)

type clock interface {
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var defaultClock clock = realClock{}

var errIdempotencyFailed = errors.New("server reported idempotent operation failed")

func idempotencyInterceptorWithClock(clk clock) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set(idempotencyHeaderKey, uuid.New().String())

			deadlineCtx, cancel := context.WithTimeout(ctx, idempotencyTotalBudget)
			defer cancel()

			backoff := idempotencyInitialBackoff
			for {
				resp, err := next(deadlineCtx, req)
				if err != nil {
					return nil, err
				}
				switch resp.Header().Get(idempotencyHeaderStatus) {
				case "", statusCompleted:
					return resp, nil
				case statusFailed:
					return nil, connect.NewError(connect.CodeInternal, errIdempotencyFailed)
				}
				if err := clk.Sleep(deadlineCtx, backoff); err != nil {
					return nil, err
				}
				backoff *= 2
				if backoff > idempotencyMaxBackoff {
					backoff = idempotencyMaxBackoff
				}
			}
		}
	}
}

func (c *Client) idempotencyInterceptor() connect.UnaryInterceptorFunc {
	return idempotencyInterceptorWithClock(defaultClock)
}
