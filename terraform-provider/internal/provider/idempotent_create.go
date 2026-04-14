package provider

import (
	"context"
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
	_ = uuid.New // used in Task 2
	return nil, nil
}
