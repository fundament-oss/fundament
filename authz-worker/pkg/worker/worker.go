package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openfga/go-sdk/client"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/authz-worker/pkg/worker/handler"
	"github.com/fundament-oss/fundament/common/rollback"
)

// Config holds configuration for the outbox worker.
type Config struct {
	PollInterval time.Duration
	BatchSize    int32
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	MaxRetries   int32
}

// Worker processes the authz outbox table and syncs tuples to OpenFGA.
type Worker struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	handler *handler.Handler
	logger  *slog.Logger
	cfg     Config
}

// New creates a new authz worker with sensible defaults.
func New(pool *pgxpool.Pool, fgaClient *client.OpenFgaClient, logger *slog.Logger, cfg Config) *Worker {
	cfg = applyDefaults(cfg)

	return &Worker{
		pool:    pool,
		queries: db.New(pool),
		handler: handler.New(fgaClient, logger),
		logger:  logger,
		cfg:     cfg,
	}
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
	return cfg
}

// Run starts the worker loop. It blocks until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting authz worker",
		"poll_interval", w.cfg.PollInterval,
		"batch_size", w.cfg.BatchSize,
	)

	conn, err := w.setupListener(ctx)
	if err != nil {
		return err
	}

	defer conn.Release()

	w.processBatch(ctx)

	return w.runLoop(ctx, conn.Conn())
}

func (w *Worker) setupListener(ctx context.Context) (*pgxpool.Conn, error) {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection for LISTEN: %w", err)
	}

	if _, err := conn.Exec(ctx, "LISTEN authz_outbox"); err != nil {
		conn.Release()
		return nil, fmt.Errorf("LISTEN: %w", err)
	}

	return conn, nil
}

func (w *Worker) runLoop(ctx context.Context, conn *pgx.Conn) error {
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("shutting down authz worker")
			return nil
		default:
		}

		waitCtx, cancel := context.WithTimeout(ctx, w.cfg.PollInterval)
		_, err := conn.WaitForNotification(waitCtx)
		cancel()

		if err != nil && waitCtx.Err() == nil {
			return fmt.Errorf("wait for notification: %w", err)
		}

		w.processBatch(ctx)
	}
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
	case item.UserID.Valid:
		return w.handler.User(ctx, qtx, item.UserID.Bytes)
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
