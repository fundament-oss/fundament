package status

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

type mockStatusHandler struct {
	callCount atomic.Int32
}

func (m *mockStatusHandler) CheckStatus(_ context.Context) error {
	m.callCount.Add(1)
	return nil
}

func TestStatusWorker_RunsOnStartup(t *testing.T) {
	registry := handler.NewRegistry()
	mock := &mockStatusHandler{}
	registry.RegisterStatus(mock)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	w := New(registry, logger, Config{Interval: 1 * time.Hour})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.Run(ctx)
	}()

	// Wait for the initial poll to complete
	deadline := time.After(2 * time.Second)
	for {
		if mock.callCount.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for initial status poll")
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

func TestStatusWorker_StopsOnContextCancel(t *testing.T) {
	registry := handler.NewRegistry()
	mock := &mockStatusHandler{}
	registry.RegisterStatus(mock)

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
