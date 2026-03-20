// Package outbox implements the cluster outbox worker.
// It processes outbox rows and dispatches to entity-specific sync handlers.
package outbox

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
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"5s"`
	BaseBackoff  time.Duration `env:"BASE_BACKOFF" envDefault:"500ms"`
	MaxBackoff   time.Duration `env:"MAX_BACKOFF" envDefault:"1m"`
	MaxRetries   int32         `env:"MAX_RETRIES" envDefault:"10"`
	BackoffDelay time.Duration `env:"BACKOFF_DELAY" envDefault:"5s"`
}

// Worker processes the cluster outbox table and dispatches to handlers.
type Worker struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	registry *handler.Registry
	logger   *slog.Logger
	cfg      Config

	ready atomic.Bool
}

func New(pool *pgxpool.Pool, registry *handler.Registry, logger *slog.Logger, cfg Config) *Worker {
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	return &Worker{
		pool:     pool,
		queries:  db.New(pool),
		registry: registry,
		logger:   logger.With("worker_id", workerID, "worker", "outbox"),
		cfg:      cfg,
	}
}

// IsReady returns true if the worker is connected and processing.
func (w *Worker) IsReady() bool {
	return w.ready.Load()
}

// Run starts the worker with automatic reconnection on LISTEN connection loss.
func (w *Worker) Run(ctx context.Context) error {
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

func (w *Worker) runWithConnection(ctx context.Context) error {
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

	// Process any pending work on startup
	w.processAllRows(ctx)

	for {
		w.logger.Debug("waiting for notification or poll timeout", "poll_interval", w.cfg.PollInterval)
		notified, err := w.waitForNotification(ctx, conn)
		if err != nil {
			return err
		}

		if notified {
			w.logger.Debug("woke up from notification")
		} else {
			w.logger.Debug("woke up from poll timeout")
		}
		w.processAllRows(ctx)
	}
}

func (w *Worker) waitForNotification(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
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

// processAllRows drains all processable outbox rows in a loop.
// Row-level errors (handler failures, invalid rows) are handled inside processNextRow.
// Infrastructure errors (connection loss, commit failure) cause a backoff to avoid a
// tight retry loop.
func (w *Worker) processAllRows(ctx context.Context) {
	for {
		hasNext, err := w.processNextRow(ctx)
		if err != nil {
			w.logger.Error("failed to process outbox item", "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(w.cfg.BackoffDelay):
			}
			return
		}
		if !hasNext {
			return
		}
	}
}

func (w *Worker) processNextRow(ctx context.Context) (hasNext bool, err error) {
	if ctx.Err() != nil {
		return false, nil
	}

	// Row lock: OutboxGetAndLock uses FOR NO KEY UPDATE SKIP LOCKED,
	// so the row is locked for the lifetime of this transaction.
	// Happy path: mark processed + commit inside the same tx.
	// Error path: rollback tx (releases lock), then mark retry/failed via pool
	// to avoid deadlock (pool UPDATE would block on the tx's row lock).
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer rollback.Rollback(ctx, tx, w.logger)

	qtx := w.queries.WithTx(tx)

	row, err := qtx.OutboxGetAndLock(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.logger.Debug("no pending outbox rows")
			return false, nil
		}
		return false, fmt.Errorf("get next outbox row: %w", err)
	}

	entityType, entityID, err := entityFromRow(&row)
	if err != nil {
		w.logger.Error("invalid outbox row, marking as failed",
			"outbox_id", row.ID,
			"error", err)
		_ = tx.Rollback(ctx) // release lock before marking via pool
		markErr := w.queries.OutboxMarkFailed(ctx, db.OutboxMarkFailedParams{
			ID:         row.ID,
			StatusInfo: pgtype.Text{String: err.Error(), Valid: true},
		})
		if markErr != nil {
			return false, fmt.Errorf("mark failed for invalid entity: %w", markErr)
		}
		return true, nil
	}

	w.logger.Debug("processing outbox row",
		"outbox_id", row.ID,
		"entity_type", entityType,
		"entity_id", entityID,
		"event", row.Event,
		"source", row.Source,
		"retries", row.Retries)

	h, err := w.registry.SyncHandlerFor(entityType, row.Event)
	if err != nil {
		w.logger.Error("no handler registered, marking as failed",
			"outbox_id", row.ID,
			"entity_type", entityType,
			"error", err)
		_ = tx.Rollback(ctx)
		markErr := w.queries.OutboxMarkFailed(ctx, db.OutboxMarkFailedParams{
			ID:         row.ID,
			StatusInfo: pgtype.Text{String: err.Error(), Valid: true},
		})
		if markErr != nil {
			return false, fmt.Errorf("mark failed for unhandled entity: %w", markErr)
		}
		return true, nil
	}

	err = h.Sync(ctx, entityID, handler.SyncContext{EntityType: entityType, Event: row.Event, Source: row.Source})
	if err != nil {
		w.logger.Warn("handler returned error",
			"outbox_id", row.ID,
			"entity_type", entityType,
			"error", err)
		_ = tx.Rollback(ctx) // release lock before marking via pool
		markErr := w.handleRowError(ctx, w.queries, &row, err)
		if markErr != nil {
			return false, fmt.Errorf("handle processing error: %w", markErr)
		}
		return true, nil
	}

	// Happy path: mark processed and commit in the same transaction.
	err = qtx.OutboxMarkProcessed(ctx, db.OutboxMarkProcessedParams{ID: row.ID})
	if err != nil {
		return false, fmt.Errorf("mark as processed: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	w.logger.Debug("outbox row processed",
		"outbox_id", row.ID,
		"entity_type", entityType,
		"entity_id", entityID)

	return true, nil
}

func (w *Worker) handleRowError(ctx context.Context, qtx *db.Queries, row *db.OutboxGetAndLockRow, processErr error) error {
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

// entityFromRow determines the entity type and ID from the outbox row's FK columns.
// Exactly one FK column is non-null (enforced by the num_nonnulls check constraint).
func entityFromRow(row *db.OutboxGetAndLockRow) (handler.EntityType, uuid.UUID, error) {
	switch {
	case row.ClusterID.Valid:
		return handler.EntityCluster, uuid.UUID(row.ClusterID.Bytes), nil
	case row.OrganizationUserID.Valid:
		return handler.EntityOrgUser, uuid.UUID(row.OrganizationUserID.Bytes), nil
	case row.ProjectMemberID.Valid:
		return handler.EntityProjectMember, uuid.UUID(row.ProjectMemberID.Bytes), nil
	default:
		return "", uuid.Nil, fmt.Errorf("no valid entity FK in outbox row %s", row.ID)
	}
}

func durationToInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{
		Microseconds: d.Microseconds(),
		Valid:        true,
	}
}
