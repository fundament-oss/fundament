package reconcile

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

type mockReconcileHandler struct {
	callCount atomic.Int32
}

func (m *mockReconcileHandler) Reconcile(_ context.Context) error {
	m.callCount.Add(1)
	return nil
}

func TestReconcileWorker_RunsImmediately(t *testing.T) {
	registry := handler.NewRegistry()
	mock := &mockReconcileHandler{}
	registry.RegisterReconcile(mock)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	w := New(registry, logger, Config{Interval: 1 * time.Hour})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.Run(ctx)
	}()

	// Wait for the initial reconcile to complete
	deadline := time.After(2 * time.Second)
	for mock.callCount.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for initial reconcile")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done

	if count := mock.callCount.Load(); count < 1 {
		t.Errorf("expected at least 1 call, got %d", count)
	}
}

func TestReconcileWorker_StopsOnContextCancel(t *testing.T) {
	registry := handler.NewRegistry()
	mock := &mockReconcileHandler{}
	registry.RegisterReconcile(mock)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	w := New(registry, logger, Config{Interval: 1 * time.Hour})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.Run(ctx)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error on context cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker to stop")
	}
}
