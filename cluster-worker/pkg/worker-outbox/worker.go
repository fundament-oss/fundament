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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/rollback"
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

func (w *OutboxWorker) processAllRows(ctx context.Context) {
	for {
		found, err := w.processNextRow(ctx)
		if err != nil {
			w.logger.Error("failed to process outbox item", "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(w.cfg.BackoffDelay):
			}
			return
		}
		if !found {
			return
		}
	}
}

func (w *OutboxWorker) processNextRow(ctx context.Context) (found bool, err error) {
	if ctx.Err() != nil {
		return false, nil
	}

	// Row lock acquired: OutboxGetAndLock uses FOR NO KEY UPDATE SKIP LOCKED,
	// so the row is locked for the lifetime of this transaction.
	// Row lock released: on tx.Commit() or defer rollback.Rollback().
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer rollback.Rollback(ctx, tx, w.logger)

	qtx := w.queries.WithTx(tx)

	row, err := qtx.OutboxGetAndLock(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("get next outbox row: %w", err)
	}

	entityType, entityID := entityFromRow(&row)

	w.logger.Debug("processing outbox row",
		"outbox_id", row.ID,
		"entity_type", entityType,
		"entity_id", entityID,
		"event", row.Event,
		"retries", row.Retries)

	h, err := w.registry.SyncHandlerFor(entityType)
	if err != nil {
		w.logger.Error("no handler registered, marking as failed",
			"outbox_id", row.ID,
			"entity_type", entityType,
			"error", err)
		if markErr := qtx.OutboxMarkFailed(ctx, db.OutboxMarkFailedParams{
			ID:         row.ID,
			StatusInfo: pgtype.Text{String: err.Error(), Valid: true},
		}); markErr != nil {
			return true, fmt.Errorf("mark failed for unhandled entity: %w", markErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return true, fmt.Errorf("commit after marking unhandled entity failed: %w", commitErr)
		}
		return true, nil
	}

	if syncErr := h.Sync(ctx, entityID); syncErr != nil {
		if err := w.handleRowError(ctx, qtx, &row, syncErr); err != nil {
			return true, fmt.Errorf("handle processing error: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return true, fmt.Errorf("commit after error: %w", err)
		}
		return true, nil
	}

	if err := qtx.OutboxMarkProcessed(ctx, db.OutboxMarkProcessedParams{ID: row.ID}); err != nil {
		return true, fmt.Errorf("mark as processed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return true, fmt.Errorf("commit: %w", err)
	}

	w.logger.Debug("outbox row processed",
		"outbox_id", row.ID,
		"entity_type", entityType,
		"entity_id", entityID)

	return true, nil
}

func (w *OutboxWorker) handleRowError(ctx context.Context, qtx *db.Queries, row *db.OutboxGetAndLockRow, processErr error) error {
	statusInfo := pgtype.Text{String: processErr.Error(), Valid: true}

	// Check if we've exceeded max retries. row.Retries is the current count
	// before this failure, so +1 is the count after this attempt.
	if row.Retries+1 >= w.cfg.MaxRetries {
		w.logger.Error("outbox item exceeded max retries, marking as failed",
			"outbox_id", row.ID,
			"retries", row.Retries+1,
			"max_retries", w.cfg.MaxRetries,
			"error", processErr)

		if err := qtx.OutboxMarkFailed(ctx, db.OutboxMarkFailedParams{
			ID:         row.ID,
			StatusInfo: statusInfo,
		}); err != nil {
			return fmt.Errorf("mark outbox failed: %w", err)
		}
		return nil
	}

	retries, err := qtx.OutboxMarkRetry(ctx, db.OutboxMarkRetryParams{
		ID:           row.ID,
		BaseInterval: durationToInterval(w.cfg.BaseBackoff),
		MaxBackoff:   durationToInterval(w.cfg.MaxBackoff),
		StatusInfo:   statusInfo,
	})
	if err != nil {
		return fmt.Errorf("mark outbox retry: %w", err)
	}

	w.logger.Warn("failed to process outbox item, will retry",
		"outbox_id", row.ID,
		"retries", retries,
		"error", processErr)

	return nil
}

// reconcile delegates reconciliation to each registered ReconcileHandler.
// Each handler owns its own re-enqueue and orphan-detection logic.
func (w *OutboxWorker) reconcileAllHandlers(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	w.logger.Info("starting outbox reconciliation")

	for _, h := range w.registry.ReconcileHandlers() {
		if err := h.Reconcile(ctx); err != nil {
			w.logger.Error("reconcile handler failed", "error", err)
		}
	}

	w.logger.Info("outbox reconciliation complete")
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
