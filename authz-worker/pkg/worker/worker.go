package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openfga/go-sdk/client"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/authz-worker/pkg/worker/handler"
	"github.com/fundament-oss/fundament/common/rollback"
)

const listenChannel = "authz_outbox"

// Config holds configuration for the outbox worker.
type Config struct {
	PollInterval time.Duration
	BatchSize    int32
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	MaxRetries   int32
	BackoffDelay time.Duration
}

// Worker processes the authz outbox table and syncs tuples to OpenFGA.
type Worker struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	handler *handler.Handler
	logger  *slog.Logger
	cfg     Config
	ready   atomic.Bool
}

// New creates a new authz worker with sensible defaults.
func New(pool *pgxpool.Pool, fgaClient *client.OpenFgaClient, logger *slog.Logger, cfg Config) *Worker {
	cfg = applyDefaults(cfg)

	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	return &Worker{
		pool:    pool,
		queries: db.New(pool),
		handler: handler.New(fgaClient, logger),
		logger:  logger.With("worker_id", workerID),
		cfg:     cfg,
	}
}

// IsReady returns whether the worker has an active LISTEN connection and is processing.
func (w *Worker) IsReady() bool {
	return w.ready.Load()
}

func applyDefaults(cfg Config) Config {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.BaseBackoff == 0 {
		cfg.BaseBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 1 * time.Minute
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.BackoffDelay == 0 {
		cfg.BackoffDelay = 5 * time.Second
	}
	return cfg
}

// Run starts the worker with automatic reconnection. It blocks until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting authz worker",
		"poll_interval", w.cfg.PollInterval,
		"batch_size", w.cfg.BatchSize,
	)

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
	conn, err := w.setupListener(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	w.ready.Store(true)

	// Reset permanently-failed items so they are retried after a worker restart.
	if err := w.queries.ResetFailedOutboxItems(ctx); err != nil {
		return fmt.Errorf("reset failed outbox items: %w", err)
	}

	w.processBatch(ctx)

	for {
		notified, err := w.waitForNotification(ctx, conn)
		if err != nil {
			return err
		}
		if notified {
			w.processBatch(ctx)
		}
	}
}

func (w *Worker) setupListener(ctx context.Context) (*pgxpool.Conn, error) {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection for LISTEN: %w", err)
	}

	if _, err := conn.Exec(ctx, "LISTEN "+listenChannel); err != nil {
		conn.Release()
		return nil, fmt.Errorf("LISTEN: %w", err)
	}

	w.logger.Info("listening for authz_outbox notifications")

	return conn, nil
}

func (w *Worker) waitForNotification(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	waitCtx, cancel := context.WithTimeout(ctx, w.cfg.PollInterval)
	defer cancel()

	_, err := conn.Conn().WaitForNotification(waitCtx)

	switch {
	case err == nil:
		return true, nil
	case errors.Is(ctx.Err(), context.Canceled):
		return false, fmt.Errorf("shutdown requested: %w", ctx.Err())
	case errors.Is(err, context.DeadlineExceeded):
		// Active health check: verify the connection is alive and still listening.
		// IsClosed() alone is insufficient — TCP connections can be silently dead
		// (firewall drops, network partitions) while IsClosed() still returns false.
		if err := w.verifyConnection(ctx, conn); err != nil {
			return false, err
		}
		return false, nil

	case conn.Conn().IsClosed():
		return false, fmt.Errorf("connection closed")

	default:
		w.logger.Warn("unexpected error waiting for notification", "error", err)
		return false, nil
	}
}

func (w *Worker) verifyConnection(ctx context.Context, conn *pgxpool.Conn) error {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Step 1: Verify the connection is alive.
	if err := conn.Conn().Ping(checkCtx); err != nil {
		return fmt.Errorf("connection health check failed: %w", err)
	}

	// Step 2: Verify the LISTEN subscription is still active.
	rows, err := conn.Query(checkCtx, "SELECT pg_listening_channels()")
	if err != nil {
		return fmt.Errorf("failed to query listening channels: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return fmt.Errorf("failed to scan listening channel: %w", err)
		}
		if channel == listenChannel {
			return nil
		}
	}

	return fmt.Errorf("LISTEN subscription lost for channel %q", listenChannel)
}

func (w *Worker) processBatch(ctx context.Context) {
	for {
		processed := w.processOneBatch(ctx)
		if processed == 0 {
			return
		}

		w.logger.Debug("processed outbox batch", "count", processed)
	}
}

func (w *Worker) processOneBatch(ctx context.Context) (processed int) {
	for range w.cfg.BatchSize {
		found, err := w.processOneItem(ctx)
		if err != nil {
			w.logger.Error("failed to process outbox item", "error", err)
		}

		if !found {
			break
		}

		processed++
	}

	return processed
}

func (w *Worker) processOneItem(ctx context.Context) (found bool, err error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}

	defer rollback.Rollback(ctx, tx, w.logger)

	qtx := w.queries.WithTx(tx)

	item, err := qtx.GetAndLockNextOutboxRow(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("get next outbox row: %w", err)
	}

	if err := w.dispatchItem(ctx, qtx, &item); err != nil {
		if err := w.handleProcessingError(ctx, qtx, &item, err); err != nil {
			return true, fmt.Errorf("handle processing error: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return true, fmt.Errorf("dispatch item commit: %w", err)
		}

		return true, err
	}

	if err := qtx.MarkOutboxRowProcessed(ctx, db.MarkOutboxRowProcessedParams{ID: item.ID}); err != nil {
		return true, fmt.Errorf("mark as processed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return true, fmt.Errorf("commit: %w", err)
	}

	return true, nil
}

func (w *Worker) handleProcessingError(ctx context.Context, qtx *db.Queries, item *db.GetAndLockNextOutboxRowRow, processErr error) error {
	statusInfo := pgtype.Text{String: processErr.Error(), Valid: true}

	retries, err := qtx.MarkOutboxRowRetry(ctx, db.MarkOutboxRowRetryParams{
		ID:           item.ID,
		BaseInterval: durationToInterval(w.cfg.BaseBackoff),
		MaxBackoff:   durationToInterval(w.cfg.MaxBackoff),
		StatusInfo:   statusInfo,
	})
	if err != nil {
		return fmt.Errorf("mark outbox retry: %w", err)
	}

	if retries >= w.cfg.MaxRetries {
		w.logger.Error("outbox item exceeded max retries, marking as failed",
			"id", item.ID,
			"retries", retries,
			"max_retries", w.cfg.MaxRetries,
			"error", processErr,
		)

		if err := qtx.MarkOutboxRowFailed(ctx, db.MarkOutboxRowFailedParams{
			ID:         item.ID,
			StatusInfo: statusInfo,
		}); err != nil {
			return fmt.Errorf("mark outbox failed: %w", err)
		}
	} else {
		w.logger.Warn("failed to process outbox item, will retry",
			"id", item.ID,
			"retries", retries,
			"error", processErr,
		)
	}

	return nil
}

func (w *Worker) dispatchItem(ctx context.Context, qtx *db.Queries, item *db.GetAndLockNextOutboxRowRow) error {
	switch {
	case item.OrganizationUserID.Valid:
		return w.handler.OrganizationUser(ctx, qtx, item.OrganizationUserID.Bytes)
	case item.ProjectID.Valid:
		return w.handler.Project(ctx, qtx, item.ProjectID.Bytes)
	case item.ProjectMemberID.Valid:
		return w.handler.ProjectMember(ctx, qtx, item.ProjectMemberID.Bytes)
	case item.ClusterID.Valid:
		return w.handler.Cluster(ctx, qtx, item.ClusterID.Bytes)
	case item.NodePoolID.Valid:
		return w.handler.NodePool(ctx, qtx, item.NodePoolID.Bytes)
	case item.NamespaceID.Valid:
		return w.handler.Namespace(ctx, qtx, item.NamespaceID.Bytes)
	case item.ApiKeyID.Valid:
		return w.handler.ApiKey(ctx, qtx, item.ApiKeyID.Bytes)
	case item.InstallID.Valid:
		return w.handler.Install(ctx, qtx, item.InstallID.Bytes)
	default:
		return fmt.Errorf("unknown outbox subject FK")
	}
}

func durationToInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{
		Microseconds: d.Microseconds(),
		Valid:        true,
	}
}
