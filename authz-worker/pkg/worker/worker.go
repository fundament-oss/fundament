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
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/rollback"
)

const (
	listenChannel = "authz_outbox"

	storeCheckTimeout = 5 * time.Second

	// finalizeTimeout bounds the post-dispatch DB work (mark processed/retry
	// + commit) when running on a context that may already have been cancelled
	// by SIGTERM. The OpenFGA write has already happened at that point; if we
	// abandon the transaction the same tuple gets replayed on the next pod and
	// OpenFGA rejects it as a duplicate.
	finalizeTimeout = 10 * time.Second
)

// errStoreUnavailable lets Run distinguish a failed store check from a lost DB connection.
var errStoreUnavailable = errors.New("openfga store unavailable")

// Config holds configuration for the outbox worker.
type Config struct {
	// Generation is the release this worker belongs to. When set, a drain waits
	// until openfga reports the same generation, so seeded rows are never written
	// to a store the current release is about to replace.
	Generation   string
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
	store   *authz.ProvisionedStore
	logger  *slog.Logger
	cfg     Config
	ready   atomic.Bool

	// dispatch processes a single locked outbox item within tx. It defaults to
	// dispatchItem and exists as a field so tests can substitute failure
	// scenarios, including ones that abort tx (SQLSTATE 25P02).
	dispatch func(ctx context.Context, tx pgx.Tx, qtx *db.Queries, item *db.GetAndLockNextOutboxRowRow) error

	// verify gates each drain on the OpenFGA store. Defaults to verifyStore and
	// exists as a field so tests can stub it.
	verify func(ctx context.Context) error
}

// New creates a new authz worker with sensible defaults.
func New(
	pool *pgxpool.Pool, fgaClient *client.OpenFgaClient, store *authz.ProvisionedStore,
	logger *slog.Logger, cfg Config,
) *Worker {
	cfg = applyDefaults(cfg)

	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	w := &Worker{
		pool:    pool,
		queries: db.New(pool),
		handler: handler.New(fgaClient, store, logger),
		store:   store,
		logger:  logger.With("worker_id", workerID),
		cfg:     cfg,
	}
	w.dispatch = func(ctx context.Context, _ pgx.Tx, qtx *db.Queries, item *db.GetAndLockNextOutboxRowRow) error {
		return w.dispatchItem(ctx, qtx, item)
	}
	w.verify = w.verifyStore

	return w
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
		if errors.Is(err, errStoreUnavailable) {
			w.logger.Error("openfga store unavailable, holding outbox", "error", err, "delay", w.cfg.BackoffDelay)
		} else {
			w.logger.Error("connection lost, reconnecting", "error", err, "delay", w.cfg.BackoffDelay)
		}
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

	// Ready only once the store checks out — a worker without its store makes no progress.
	if err := w.verify(ctx); err != nil {
		return err
	}

	w.ready.Store(true)

	// Reset permanently-failed items so they are retried after a worker restart.
	if err := w.queries.ResetFailedOutboxItems(ctx); err != nil {
		return fmt.Errorf("reset failed outbox items: %w", err)
	}

	w.processBatch(ctx)

	for {
		// The bool reports whether a NOTIFY arrived (true) or the poll interval
		// elapsed (false); we drain the outbox on both. Polling matters because
		// the outbox_notify trigger only fires AFTER INSERT: a row left in
		// 'retrying' emits no NOTIFY when its retry_after elapses, so without a
		// timeout-driven pass it would lag forever and trip the consumer's
		// circuit breaker.
		if _, err := w.waitForNotification(ctx, conn); err != nil {
			return err
		}
		if err := w.verify(ctx); err != nil {
			return err
		}
		w.processBatch(ctx)
	}
}

// verifyStore gates a drain on the datastore being this release's.
//
// A reset leaves the outgoing store in place until the wipe runs, and OpenFGA's
// Write does not check that a store survives, so a drain against it reports
// success and marks rows completed for tuples about to be destroyed. The
// generation is what tells the two apart.
func (w *Worker) verifyStore(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, storeCheckTimeout)
	defer cancel()

	reported, err := w.store.Generation(checkCtx)
	if err != nil {
		return fmt.Errorf("%w: %w", errStoreUnavailable, err)
	}

	if w.cfg.Generation != "" && reported != w.cfg.Generation {
		return fmt.Errorf("%w: openfga serves generation %q, this release is %q",
			errStoreUnavailable, reported, w.cfg.Generation)
	}

	return nil
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

	dispatchErr := w.dispatch(ctx, tx, qtx, &item)

	// Past this point the side effect on OpenFGA has already happened (success
	// or partial), so the outbox row MUST be marked + committed even if the
	// parent ctx was cancelled mid-flight. Run the remainder on a detached
	// context with a bounded timeout.
	finalizeCtx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer cancel()

	if dispatchErr != nil {
		// dispatchItem may have failed on a SQL statement, which aborts the
		// transaction (SQLSTATE 25P02) and makes it unusable for any further
		// query. Roll back to release the row lock, then record the retry or
		// failure via a fresh pool connection instead of the aborted tx.
		if err := tx.Rollback(finalizeCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return true, fmt.Errorf("rollback after dispatch error: %w", err)
		}

		if err := w.handleProcessingError(finalizeCtx, w.queries, &item, dispatchErr); err != nil {
			return true, fmt.Errorf("handle processing error: %w", err)
		}

		return true, dispatchErr
	}

	if err := qtx.MarkOutboxRowProcessed(finalizeCtx, db.MarkOutboxRowProcessedParams{ID: item.ID}); err != nil {
		return true, fmt.Errorf("mark as processed: %w", err)
	}

	if err := tx.Commit(finalizeCtx); err != nil {
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
	case item.PluginID.Valid:
		return w.handler.Plugin(ctx, qtx, item.PluginID.Bytes)
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
