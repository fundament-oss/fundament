// Package outbox implements the cluster outbox worker.
// It processes outbox rows and dispatches to entity-specific sync handlers.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
)

// Config holds configuration for the outbox worker.
type Config struct {
	PollInterval             time.Duration `env:"POLL_INTERVAL" envDefault:"5s"`
	BaseBackoff              time.Duration `env:"BASE_BACKOFF" envDefault:"500ms"`
	MaxBackoff               time.Duration `env:"MAX_BACKOFF" envDefault:"1m"`
	MaxRetries               int32         `env:"MAX_RETRIES" envDefault:"10"`
	BackoffDelay             time.Duration `env:"BACKOFF_DELAY" envDefault:"5s"`
	PreconditionDelay        time.Duration `env:"PRECONDITION_DELAY" envDefault:"30s"`
	MaxPreconditionDeferrals int32         `env:"MAX_PRECONDITION_DEFERRALS" envDefault:"100"`
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

// processNextRow is a thin orchestrator: claim → process → complete.
func (w *Worker) processNextRow(ctx context.Context) (hasNext bool, err error) {
	if ctx.Err() != nil {
		return false, nil
	}

	row, tx, err := w.claim(ctx)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	defer rollback.Rollback(ctx, tx, w.logger)

	entityType, processErr := w.process(ctx, row)

	return w.complete(ctx, row, tx, entityType, processErr)
}

// claim begins a transaction and locks the next pending outbox row.
// Returns nil row if no rows are available.
func (w *Worker) claim(ctx context.Context) (*db.OutboxGetAndLockRow, pgx.Tx, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}

	qtx := w.queries.WithTx(tx)
	row, err := qtx.OutboxGetAndLock(ctx)
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			w.logger.Debug("no pending outbox rows")
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("get next outbox row: %w", err)
	}

	return &row, tx, nil
}

// process extracts the entity, finds the handler, and dispatches.
// Returns the entity type (for logging) and any handler error.
func (w *Worker) process(ctx context.Context, row *db.OutboxGetAndLockRow) (handler.EntityType, error) {
	entityType, entityID, err := entityFromRow(row)
	if err != nil {
		return "", err
	}

	event := dbconst.ClusterOutboxEvent(row.Event)
	source := dbconst.ClusterOutboxSource(row.Source)

	w.logger.Debug("processing outbox row",
		"outbox_id", row.ID,
		"entity_type", entityType,
		"entity_id", entityID,
		"event", event,
		"source", source,
		"retries", row.Retries)

	h, err := w.registry.SyncHandlerFor(entityType, event)
	if err != nil {
		return entityType, fmt.Errorf("lookup handler: %w", err)
	}

	if err := h.Sync(ctx, entityID, handler.SyncContext{EntityType: entityType, Event: event, Source: source}); err != nil {
		return entityType, fmt.Errorf("sync %s %s: %w", entityType, entityID, err)
	}
	return entityType, nil
}

// complete finalizes the outbox row based on the processing result.
// On success: mark processed + commit inside the same tx.
// On PreconditionError: rollback tx, defer without retry increment.
// On other error: rollback tx (releases lock), then mark retry/failed via pool.
func (w *Worker) complete(ctx context.Context, row *db.OutboxGetAndLockRow, tx pgx.Tx, entityType handler.EntityType, processErr error) (bool, error) {
	if processErr != nil {
		var precondErr *handler.PreconditionError
		if errors.As(processErr, &precondErr) {
			return w.handlePreconditionError(ctx, row, tx, entityType, precondErr)
		}

		w.logger.Warn("handler returned error",
			"outbox_id", row.ID,
			"entity_type", entityType,
			"error", processErr)
		_ = tx.Rollback(ctx) // release lock before marking via pool
		markErr := w.handleRowError(ctx, w.queries, row, processErr)
		if markErr != nil {
			return false, fmt.Errorf("handle processing error: %w", markErr)
		}
		return true, nil
	}

	// Happy path: mark processed and commit in the same transaction.
	qtx := w.queries.WithTx(tx)
	if err := qtx.OutboxMarkProcessed(ctx, db.OutboxMarkProcessedParams{ID: row.ID}); err != nil {
		return false, fmt.Errorf("mark as processed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	entityID := uuid.Nil
	if entityType != "" {
		_, entityID, _ = entityFromRow(row)
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
	case row.NodePoolID.Valid:
		return handler.EntityNodePool, uuid.UUID(row.NodePoolID.Bytes), nil
	default:
		return "", uuid.Nil, fmt.Errorf("no valid entity FK in outbox row %s", row.ID)
	}
}

// handlePreconditionError defers a row without incrementing retries.
// If the deferral count exceeds MaxPreconditionDeferrals, it falls through to
// regular error handling (increments retries, applies backoff/fail logic).
func (w *Worker) handlePreconditionError(ctx context.Context, row *db.OutboxGetAndLockRow, tx pgx.Tx, entityType handler.EntityType, precondErr *handler.PreconditionError) (bool, error) {
	_ = tx.Rollback(ctx) // release lock before deferring via pool

	deferrals := parseDeferralCount(row.StatusInfo.String) + 1

	if deferrals > w.cfg.MaxPreconditionDeferrals {
		w.logger.Warn("precondition deferral cap exceeded, treating as regular error",
			"outbox_id", row.ID,
			"entity_type", entityType,
			"deferrals", deferrals,
			"reason", precondErr.Reason)
		markErr := w.handleRowError(ctx, w.queries, row, precondErr)
		if markErr != nil {
			return false, fmt.Errorf("handle processing error: %w", markErr)
		}
		return true, nil
	}

	statusInfo := fmt.Sprintf("precondition_deferrals=%d; %s", deferrals, precondErr.Reason)

	if err := w.queries.OutboxDeferWithoutRetry(ctx, db.OutboxDeferWithoutRetryParams{
		ID:         row.ID,
		Delay:      durationToInterval(w.cfg.PreconditionDelay),
		StatusInfo: pgtype.Text{String: statusInfo, Valid: true},
	}); err != nil {
		return false, fmt.Errorf("defer outbox row: %w", err)
	}

	w.logger.Debug("deferred outbox row (precondition not met)",
		"outbox_id", row.ID,
		"entity_type", entityType,
		"deferrals", deferrals,
		"reason", precondErr.Reason)

	return true, nil
}

var deferralCountRe = regexp.MustCompile(`^precondition_deferrals=(\d+);`)

// parseDeferralCount extracts the precondition deferral count from status_info.
func parseDeferralCount(statusInfo string) int32 {
	m := deferralCountRe.FindStringSubmatch(statusInfo)
	if m == nil {
		return 0
	}
	n, err := strconv.ParseInt(m[1], 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func durationToInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{
		Microseconds: d.Microseconds(),
		Valid:        true,
	}
}
