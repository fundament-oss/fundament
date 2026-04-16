package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
)

type fakeClock struct {
	sleeps []time.Duration
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.sleeps = append(c.sleeps, d)
	return nil
}

type fakeReq struct{}
type fakeResp struct{}

type scriptStep struct {
	status string
	err    error
}

func scriptedNext(t *testing.T, steps []scriptStep, gotKeys *[]string) connect.UnaryFunc {
	t.Helper()
	i := 0
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if i >= len(steps) {
			t.Fatalf("next invoked %d times, only %d scripted", i+1, len(steps))
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

func invoke(t *testing.T, clk clock, next connect.UnaryFunc) (connect.AnyResponse, error) {
	t.Helper()
	return invokeCtx(t, context.Background(), clk, next)
}

func invokeCtx(t *testing.T, ctx context.Context, clk clock, next connect.UnaryFunc) (connect.AnyResponse, error) {
	t.Helper()
	interceptor := idempotencyInterceptorWithClock(clk)
	wrapped := interceptor(next)
	return wrapped(ctx, connect.NewRequest(&fakeReq{}))
}

func TestIdempotencyInterceptor_CompletedOnFirstCall(t *testing.T) {
	t.Parallel()
	var keys []string
	clk := &fakeClock{}
	next := scriptedNext(t, []scriptStep{{status: statusCompleted}}, &keys)

	resp, err := invoke(t, clk, next)
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
	if len(clk.sleeps) != 0 {
		t.Fatalf("expected no sleeps on immediate completion, got %v", clk.sleeps)
	}
}

func TestIdempotencyInterceptor_MissingStatusPassesThrough(t *testing.T) {
	t.Parallel()
	var keys []string
	clk := &fakeClock{}
	next := scriptedNext(t, []scriptStep{{status: ""}}, &keys)

	resp, err := invoke(t, clk, next)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if len(keys) != 1 {
		t.Fatalf("expected exactly 1 call (no polling), got %d", len(keys))
	}
	if len(clk.sleeps) != 0 {
		t.Fatalf("expected no sleeps when status header absent, got %v", clk.sleeps)
	}
}

func TestIdempotencyInterceptor_ProcessingThenCompleted(t *testing.T) {
	t.Parallel()
	var keys []string
	clk := &fakeClock{}
	next := scriptedNext(t, []scriptStep{
		{status: statusProcessing},
		{status: statusCompleted},
	}, &keys)

	resp, err := invoke(t, clk, next)
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
		t.Fatalf("expected same idempotency key across polls, got %q and %q", keys[0], keys[1])
	}
	if len(clk.sleeps) != 1 || clk.sleeps[0] != idempotencyInitialBackoff {
		t.Fatalf("expected one 100ms sleep, got %v", clk.sleeps)
	}
}

func TestIdempotencyInterceptor_PendingAndRetryingAlsoPoll(t *testing.T) {
	t.Parallel()
	var keys []string
	clk := &fakeClock{}
	next := scriptedNext(t, []scriptStep{
		{status: statusPending},
		{status: statusRetrying},
		{status: statusCompleted},
	}, &keys)

	if _, err := invoke(t, clk, next); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(keys))
	}
	if len(clk.sleeps) != 2 {
		t.Fatalf("expected 2 sleeps, got %v", clk.sleeps)
	}
}

func TestIdempotencyInterceptor_FailedStatusReturnsInternal(t *testing.T) {
	t.Parallel()
	var keys []string
	clk := &fakeClock{}
	next := scriptedNext(t, []scriptStep{{status: statusFailed}}, &keys)

	resp, err := invoke(t, clk, next)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatal("expected nil response on failed status")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 call, got %d", len(keys))
	}
}

func TestIdempotencyInterceptor_TransportErrorPropagates(t *testing.T) {
	t.Parallel()
	var keys []string
	clk := &fakeClock{}
	wantErr := connect.NewError(connect.CodePermissionDenied, errors.New("denied"))
	next := scriptedNext(t, []scriptStep{{err: wantErr}}, &keys)

	_, err := invoke(t, clk, next)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("expected PermissionDenied to propagate, got %v", connect.CodeOf(err))
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", len(keys))
	}
}

func TestIdempotencyInterceptor_BackoffSchedule(t *testing.T) {
	t.Parallel()
	steps := []scriptStep{
		{status: statusProcessing},
		{status: statusProcessing},
		{status: statusProcessing},
		{status: statusProcessing},
		{status: statusProcessing},
		{status: statusProcessing},
		{status: statusCompleted},
	}
	var keys []string
	clk := &fakeClock{}
	if _, err := invoke(t, clk, scriptedNext(t, steps, &keys)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		2 * time.Second,
	}
	if len(clk.sleeps) != len(want) {
		t.Fatalf("expected %d sleeps, got %d (%v)", len(want), len(clk.sleeps), clk.sleeps)
	}
	for i, d := range want {
		if clk.sleeps[i] != d {
			t.Errorf("sleep[%d] = %v, want %v", i, clk.sleeps[i], d)
		}
	}
}

func TestIdempotencyInterceptor_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	steps := make([]scriptStep, 100)
	for i := range steps {
		steps[i] = scriptStep{status: statusProcessing}
	}
	var keys []string
	_, err := invokeCtx(t, ctx, defaultClock, scriptedNext(t, steps, &keys))
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
	if ctx.Err() == nil {
		t.Fatal("expected parent ctx to be done")
	}
}
