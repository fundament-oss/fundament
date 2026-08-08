package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
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

// scriptedCall returns a pre-canned sequence of (status, err) pairs.
type scriptStep struct {
	status string // value to set in Idempotency-Status response header; "" means unset
	err    error
}

const scriptedProcedure = "/test.v1.ScriptedService/Create"

// scriptedCall serves the scripted steps from an in-process Connect server and
// returns a plain client call with the same signature as generated clients, so
// the idempotency headers travel through the real call-info plumbing. The
// ExchangeToken message types are arbitrary: the wire needs some generated
// proto, but only the headers matter here.
func scriptedCall(t *testing.T, steps []scriptStep, gotKeys *[]string) func(context.Context, *authnv1.ExchangeTokenRequest) (*authnv1.ExchangeTokenResponse, error) {
	t.Helper()
	var mu sync.Mutex
	i := 0
	handler := connect.NewUnaryHandlerSimple(
		scriptedProcedure,
		func(ctx context.Context, req *authnv1.ExchangeTokenRequest) (*authnv1.ExchangeTokenResponse, error) {
			mu.Lock()
			defer mu.Unlock()
			if i >= len(steps) {
				t.Errorf("call invoked %d times, only %d scripted", i+1, len(steps))
				return nil, connect.NewError(connect.CodeInternal, errorString("call overran script"))
			}
			callInfo, ok := connect.CallInfoForHandlerContext(ctx)
			if !ok {
				t.Error("expected handler call info in context")
				return nil, connect.NewError(connect.CodeInternal, errorString("no call info"))
			}
			*gotKeys = append(*gotKeys, callInfo.RequestHeader().Get(idempotencyHeaderKey))
			step := steps[i]
			i++
			if step.err != nil {
				return nil, step.err
			}
			if step.status != "" {
				callInfo.ResponseHeader().Set(idempotencyHeaderStatus, step.status)
			}
			return &authnv1.ExchangeTokenResponse{}, nil
		},
	)
	mux := http.NewServeMux()
	mux.Handle(scriptedProcedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := connect.NewClient[authnv1.ExchangeTokenRequest, authnv1.ExchangeTokenResponse](srv.Client(), srv.URL+scriptedProcedure)
	return func(ctx context.Context, req *authnv1.ExchangeTokenRequest) (*authnv1.ExchangeTokenResponse, error) {
		resp, err := client.CallUnary(ctx, connect.NewRequest(req))
		if resp != nil {
			return resp.Msg, err //nolint:wrapcheck // tests assert on the raw connect error
		}
		return nil, err //nolint:wrapcheck // tests assert on the raw connect error
	}
}

func TestCreateIdempotent_CompletedOnFirstCall(t *testing.T) {
	var keys []string
	call := scriptedCall(t, []scriptStep{{status: statusCompleted}}, &keys)

	resp, err := createIdempotentWithClock(context.Background(), newFakeClock(), call, &authnv1.ExchangeTokenRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, keys, 1)
	assert.NotEmpty(t, keys[0], "expected Idempotency-Key header to be set")
}

func TestCreateIdempotent_ProcessingThenCompleted(t *testing.T) {
	var keys []string
	call := scriptedCall(t, []scriptStep{
		{status: statusProcessing},
		{status: statusCompleted},
	}, &keys)

	clk := newFakeClock()
	resp, err := createIdempotentWithClock(context.Background(), clk, call, &authnv1.ExchangeTokenRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, keys, 2)
	assert.Equal(t, keys[0], keys[1], "expected same idempotency key on both calls")
	assert.Equal(t, []time.Duration{idempotencyInitialBackoff}, clk.sleeps, "expected one 100ms sleep")
}

func TestCreateIdempotent_FailedStatusReturnsError(t *testing.T) {
	var keys []string
	call := scriptedCall(t, []scriptStep{{status: statusFailed}}, &keys)

	resp, err := createIdempotentWithClock(context.Background(), newFakeClock(), call, &authnv1.ExchangeTokenRequest{})
	require.Error(t, err)
	assert.Nil(t, resp, "expected nil response on failed status")
	assert.Len(t, keys, 1)
}

func TestCreateIdempotent_TransportErrorReturnsImmediately(t *testing.T) {
	var keys []string
	wantErr := connect.NewError(connect.CodePermissionDenied, errorString("no"))
	call := scriptedCall(t, []scriptStep{{err: wantErr}}, &keys)

	_, err := createIdempotentWithClock(context.Background(), newFakeClock(), call, &authnv1.ExchangeTokenRequest{})
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.Len(t, keys, 1, "expected 1 call (no retries)")
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestCreateIdempotent_BackoffSchedule(t *testing.T) {
	// Six processing calls then completed -> six sleeps with exponential
	// backoff capped at 2s: 100ms, 200ms, 400ms, 800ms, 1.6s, 2s.
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
	clk := newFakeClock()
	_, err := createIdempotentWithClock(context.Background(), clk, scriptedCall(t, steps, &keys), &authnv1.ExchangeTokenRequest{})
	require.NoError(t, err)

	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		2 * time.Second,
	}
	assert.Equal(t, want, clk.sleeps)
}

func TestCreateIdempotent_DeadlineExceeded(t *testing.T) {
	// Always processing — helper must give up when the ctx budget expires.
	// Use a ctx with a short deadline so the real clock's Sleep exits promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var keys []string
	steps := make([]scriptStep, 100)
	for i := range steps {
		steps[i] = scriptStep{status: statusProcessing}
	}
	_, err := createIdempotent(ctx, scriptedCall(t, steps, &keys), &authnv1.ExchangeTokenRequest{})
	require.Error(t, err, "expected deadline error")
	require.Error(t, ctx.Err(), "expected parent ctx to be done")
}
