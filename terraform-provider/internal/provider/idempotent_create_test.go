package provider

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
)

// fakeClock lets tests step time forward deterministically.
type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return nil
}

// fakeReq is the unconstrained request message the helper accepts.
type fakeReq struct{}
type fakeResp struct{}

// scriptedCall returns a pre-canned sequence of (status, err) pairs.
type scriptStep struct {
	status string // value to set in Idempotency-Status response header; "" means unset
	err    error
}

func scriptedCall(t *testing.T, steps []scriptStep, gotKeys *[]string) func(context.Context, *connect.Request[fakeReq]) (*connect.Response[fakeResp], error) {
	t.Helper()
	i := 0
	return func(ctx context.Context, req *connect.Request[fakeReq]) (*connect.Response[fakeResp], error) {
		if i >= len(steps) {
			t.Fatalf("call invoked %d times, only %d scripted", i+1, len(steps))
		}
		*gotKeys = append(*gotKeys, req.Header().Get(idempotencyHeaderKey))
		step := steps[i]
		i++
		if step.err != nil {
			return nil, step.err
		}
		resp := connect.NewResponse(&fakeResp{})
		if step.status != "" {
			resp.Header().Set(idempotencyHeaderStatus, step.status)
		}
		return resp, nil
	}
}

func TestCreateIdempotent_CompletedOnFirstCall(t *testing.T) {
	var keys []string
	call := scriptedCall(t, []scriptStep{{status: statusCompleted}}, &keys)

	resp, err := createIdempotentWithClock(context.Background(), newFakeClock(), call, connect.NewRequest(&fakeReq{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 call, got %d", len(keys))
	}
	if keys[0] == "" {
		t.Fatal("expected Idempotency-Key header to be set")
	}
}

func TestCreateIdempotent_ProcessingThenCompleted(t *testing.T) {
	var keys []string
	call := scriptedCall(t, []scriptStep{
		{status: statusProcessing},
		{status: statusCompleted},
	}, &keys)

	clk := newFakeClock()
	resp, err := createIdempotentWithClock(context.Background(), clk, call, connect.NewRequest(&fakeReq{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(keys))
	}
	if keys[0] != keys[1] {
		t.Fatalf("expected same idempotency key on both calls, got %q and %q", keys[0], keys[1])
	}
	if len(clk.sleeps) != 1 || clk.sleeps[0] != idempotencyInitialBackoff {
		t.Fatalf("expected one 100ms sleep, got %v", clk.sleeps)
	}
}

func TestCreateIdempotent_FailedStatusReturnsError(t *testing.T) {
	var keys []string
	call := scriptedCall(t, []scriptStep{{status: statusFailed}}, &keys)

	resp, err := createIdempotentWithClock(context.Background(), newFakeClock(), call, connect.NewRequest(&fakeReq{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatal("expected nil response on failed status")
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 call, got %d", len(keys))
	}
}

func TestCreateIdempotent_TransportErrorReturnsImmediately(t *testing.T) {
	var keys []string
	wantErr := connect.NewError(connect.CodePermissionDenied, errorString("no"))
	call := scriptedCall(t, []scriptStep{{err: wantErr}}, &keys)

	_, err := createIdempotentWithClock(context.Background(), newFakeClock(), call, connect.NewRequest(&fakeReq{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 call (no retries), got %d", len(keys))
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
