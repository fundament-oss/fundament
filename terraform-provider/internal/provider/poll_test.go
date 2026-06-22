package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPollWithBackoff(t *testing.T) {
	t.Run("succeeds after a few not-done iterations", func(t *testing.T) {
		calls := 0
		err := pollWithBackoff(context.Background(), time.Millisecond, 5, func(context.Context) (bool, bool, error) {
			calls++
			return calls >= 3, false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Fatalf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("returns fatal error immediately", func(t *testing.T) {
		sentinel := errors.New("boom")
		calls := 0
		err := pollWithBackoff(context.Background(), time.Millisecond, 5, func(context.Context) (bool, bool, error) {
			calls++
			return false, true, sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected sentinel error, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected fatal error to stop after 1 call, got %d", calls)
		}
	})

	t.Run("retries transient errors then gives up at the threshold", func(t *testing.T) {
		sentinel := errors.New("transient")
		calls := 0
		err := pollWithBackoff(context.Background(), time.Millisecond, 3, func(context.Context) (bool, bool, error) {
			calls++
			return false, false, sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected sentinel error, got %v", err)
		}
		if calls != 3 {
			t.Fatalf("expected to give up after 3 consecutive errors, got %d", calls)
		}
	})

	t.Run("resets the consecutive error counter on success", func(t *testing.T) {
		sentinel := errors.New("transient")
		calls := 0
		// Pattern: err, err, ok(not done), err, err, done.
		// Without a reset this would exceed maxConsecutiveErrors=3.
		err := pollWithBackoff(context.Background(), time.Millisecond, 3, func(context.Context) (bool, bool, error) {
			calls++
			switch calls {
			case 1, 2, 4, 5:
				return false, false, sentinel
			case 3:
				return false, false, nil
			default:
				return true, false, nil
			}
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 6 {
			t.Fatalf("expected 6 calls, got %d", calls)
		}
	})

	t.Run("returns context error on cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := pollWithBackoff(ctx, time.Hour, 5, func(context.Context) (bool, bool, error) {
			return false, false, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}
