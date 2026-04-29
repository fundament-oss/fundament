package circuitbreaker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestBreakerOpensWhenCheckTrips(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return true, nil
	})

	b.poll(context.Background())

	if !b.IsOpen() {
		t.Fatal("expected breaker to be open")
	}
}

func TestBreakerClosesWhenCheckRecovers(t *testing.T) {
	trip := true
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return trip, nil
	})

	b.poll(context.Background())
	if !b.IsOpen() {
		t.Fatal("expected breaker to be open")
	}

	trip = false
	b.poll(context.Background())
	if b.IsOpen() {
		t.Fatal("expected breaker to be closed after recovery")
	}
}

func TestBreakerStaysClosedOnCheckError(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return false, errors.New("db connection failed")
	})

	b.poll(context.Background())

	if b.IsOpen() {
		t.Fatal("expected breaker to stay closed on check error (fail-open)")
	}
}

func TestBreakerStaysClosedWhenCheckReturnsFalse(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return false, nil
	})

	b.poll(context.Background())

	if b.IsOpen() {
		t.Fatal("expected breaker to stay closed")
	}
}

func TestBreakerDefaultConfig(t *testing.T) {
	b := New(slog.Default(), Config{}, func(_ context.Context) (bool, error) {
		return false, nil
	})

	if b.config.PollInterval != 2*time.Second {
		t.Fatalf("expected default poll interval 2s, got %s", b.config.PollInterval)
	}
}

func TestBreakerStartRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := New(slog.Default(), Config{PollInterval: 10 * time.Millisecond}, func(_ context.Context) (bool, error) {
		return false, nil
	})

	done := make(chan struct{})
	go func() {
		b.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}
