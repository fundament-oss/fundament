// Package worker_outbox implements the cluster outbox worker.
// It processes outbox rows and dispatches to entity-specific sync handlers.
package worker_outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

// Config holds configuration for the outbox worker.
type Config struct {
	PollInterval      time.Duration `env:"POLL_INTERVAL" envDefault:"5s"`
	ReconcileInterval time.Duration `env:"RECONCILE_INTERVAL" envDefault:"5m"`
	BaseBackoff       time.Duration `env:"BASE_BACKOFF" envDefault:"500ms"`
	MaxBackoff        time.Duration `env:"MAX_BACKOFF" envDefault:"1m"`
	MaxRetries        int32         `env:"MAX_RETRIES" envDefault:"10"`
	BackoffDelay      time.Duration `env:"BACKOFF_DELAY" envDefault:"5s"`
}

// OutboxWorker processes the cluster outbox table and dispatches to handlers.
type OutboxWorker struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	registry *handler.Registry
	logger   *slog.Logger
	cfg      Config

	ready atomic.Bool
}

func New(pool *pgxpool.Pool, registry *handler.Registry, logger *slog.Logger, cfg Config) *OutboxWorker {
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	return &OutboxWorker{
		pool:     pool,
		queries:  db.New(pool),
		registry: registry,
		logger:   logger.With("worker_id", workerID, "worker", "outbox"),
		cfg:      cfg,
	}
}

// IsReady returns true if the worker is connected and processing.
func (w *OutboxWorker) IsReady() bool {
	return w.ready.Load()
}

// Run starts the worker with automatic reconnection on LISTEN connection loss.
func (w *OutboxWorker) Run(ctx context.Context) error {
	for {
		err := w.runWithConnection(ctx)
		if ctx.Err() != nil {
			return fmt.Errorf("worker stopped: %w", ctx.Err())
		}
		w.logger.Error("connection lost, reconnecting", "error", err, "delay", w.cfg.BackoffDelay)
		w.ready.Store(false)
		select {
		case <-ctx.Done():
			return fmt.Errorf("worker stopped: %w", ctx.Err())
		case <-time.After(w.cfg.BackoffDelay):
		}
	}
}

func (w *OutboxWorker) runWithConnection(ctx context.Context) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire listen connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN cluster_outbox"); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	w.logger.Info("listening for cluster_outbox notifications")
	w.ready.Store(true)

	// Reconcile and process any pending work on startup
	w.reconcileAllHandlers(ctx)
	w.processAllRows(ctx)
	lastReconcile := time.Now()

	for {
		_, err := w.waitForNotification(ctx, conn)
		if err != nil {
			return err
		}

		w.processAllRows(ctx)

		if time.Since(lastReconcile) >= w.cfg.ReconcileInterval {
			w.reconcileAllHandlers(ctx)
			lastReconcile = time.Now()
		}
	}
}

func (w *OutboxWorker) waitForNotification(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	waitCtx, cancel := context.WithTimeout(ctx, w.cfg.PollInterval)
	defer cancel()

	_, err := conn.Conn().WaitForNotification(waitCtx)

	switch {
	case err == nil:
		return true, nil

	case errors.Is(err, context.DeadlineExceeded):
		if ctx.Err() != nil {
			return false, fmt.Errorf("shutdown requested: %w", ctx.Err())
		}
		return false, nil

	case errors.Is(err, context.Canceled):
		return false, fmt.Errorf("shutdown requested: %w", ctx.Err())

	case conn.Conn().IsClosed():
		return false, fmt.Errorf("connection closed")

	default:
		w.logger.Warn("unexpected error waiting for notification", "error", err)
		return false, nil
	}
}

// entityFromRow returns the entity type and ID from an outbox row.
func entityFromRow(row *db.OutboxGetAndLockRow) (handler.EntityType, uuid.UUID) {
	return handler.EntityType(row.EntityType), row.SubjectID
}

func durationToInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{
		Microseconds: d.Microseconds(),
		Valid:        true,
	}
}
