package circuitbreaker

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
)

func TestUnaryPassesThroughWhenClosed(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return false, nil
	})

	i := NewInterceptor(b)
	called := false
	handler := i.WrapUnary(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	req := &fakeRequest{procedure: "/test/Foo"}
	_, _ = handler(context.Background(), req)

	if !called {
		t.Fatal("expected handler to be called when breaker is closed")
	}
}

func TestUnaryBlocksWhenOpen(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return true, nil
	})
	b.poll(context.Background())

	i := NewInterceptor(b)
	called := false
	handler := i.WrapUnary(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	req := &fakeRequest{procedure: "/test/Foo"}
	_, err := handler(context.Background(), req)

	if called {
		t.Fatal("expected handler NOT to be called when breaker is open")
	}
	if err == nil {
		t.Fatal("expected error when breaker is open")
	}
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("expected CodeUnavailable, got %v", connect.CodeOf(err))
	}
}

func TestStreamingHandlerBlocksWhenOpen(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return true, nil
	})
	b.poll(context.Background())

	i := NewInterceptor(b)
	called := false
	handler := i.WrapStreamingHandler(func(_ context.Context, _ connect.StreamingHandlerConn) error {
		called = true
		return nil
	})

	err := handler(context.Background(), nil)

	if called {
		t.Fatal("expected handler NOT to be called when breaker is open")
	}
	if err == nil {
		t.Fatal("expected error when breaker is open")
	}
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("expected CodeUnavailable, got %v", connect.CodeOf(err))
	}
}

func TestStreamingHandlerPassesThroughWhenClosed(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return false, nil
	})

	i := NewInterceptor(b)
	called := false
	handler := i.WrapStreamingHandler(func(_ context.Context, _ connect.StreamingHandlerConn) error {
		called = true
		return nil
	})

	_ = handler(context.Background(), nil)

	if !called {
		t.Fatal("expected handler to be called when breaker is closed")
	}
}

// fakeRequest implements connect.AnyRequest for testing.
type fakeRequest struct {
	connect.AnyRequest
	procedure string
}

func (r *fakeRequest) Spec() connect.Spec {
	return connect.Spec{Procedure: r.procedure}
}
