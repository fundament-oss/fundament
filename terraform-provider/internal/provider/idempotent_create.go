package provider

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

	// Outbox statuses surfaced by the server idempotency interceptor.
	// See common/idempotency/interceptor.go and the authz.outbox check
	// constraint in db/fundament.dbm.
	statusPending    = "pending"
	statusRetrying   = "retrying"
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusFailed     = "failed"

	idempotencyInitialBackoff = 100 * time.Millisecond
	idempotencyMaxBackoff     = 2 * time.Second
	idempotencyTotalBudget    = 30 * time.Second
)

// clock abstracts time for testability.
type clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

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

// createIdempotent injects an idempotency key and polls the server until the
// resource reaches a terminal outbox state.
func createIdempotent[Req, Resp any](
	ctx context.Context,
	call func(ctx context.Context, req *connect.Request[Req]) (*connect.Response[Resp], error),
	req *connect.Request[Req],
) (*connect.Response[Resp], error) {
	return createIdempotentWithClock(ctx, defaultClock, call, req)
}

func createIdempotentWithClock[Req, Resp any](
	ctx context.Context,
	clk clock,
	call func(ctx context.Context, req *connect.Request[Req]) (*connect.Response[Resp], error),
	req *connect.Request[Req],
) (*connect.Response[Resp], error) {
	key := uuid.New().String()
	req.Header().Set(idempotencyHeaderKey, key)

	deadlineCtx, cancel := context.WithTimeout(ctx, idempotencyTotalBudget)
	defer cancel()

	backoff := idempotencyInitialBackoff
	for {
		resp, err := call(deadlineCtx, req)
		if err != nil {
			return nil, err
		}

		switch resp.Header().Get(idempotencyHeaderStatus) {
		case statusCompleted:
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
