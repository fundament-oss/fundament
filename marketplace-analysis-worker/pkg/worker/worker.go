// Package worker drains appstore.plugin_image_analyses: it claims a version,
// analyzes its image and records the result (FUN-24).
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/marketplace-analysis-worker/pkg/analyze"
	db "github.com/fundament-oss/fundament/marketplace-analysis-worker/pkg/db/gen"
)

const (
	listenChannel = "plugin_image_analysis"

	// finalizeTimeout bounds recording a result on a context SIGTERM may
	// already have cancelled.
	finalizeTimeout = 10 * time.Second
)

// Analyzer is what the worker needs of analyze.Analyzer.
type Analyzer interface {
	Analyze(ctx context.Context, image string) (*analyze.Result, error)
}

// Config holds the worker's timing.
type Config struct {
	// PollInterval is the longest the worker waits without a NOTIFY. Retries
	// and expired leases emit none, so they are picked up on this beat.
	PollInterval time.Duration
	// SweepInterval is how often versions without an analysis are enqueued.
	SweepInterval time.Duration
	// JobTimeout bounds one analysis. The lease outlives it by a minute.
	JobTimeout  time.Duration
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	// MaxRetries is how many failed attempts are retried. Like every field
	// here, zero means the default (5); main refuses MAX_RETRIES below 1.
	MaxRetries   int32
	BackoffDelay time.Duration
}

// Worker processes one analysis at a time. Several replicas can run side by
// side: claims skip locked rows, and a lease covers the analysis itself.
type Worker struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	analyzer Analyzer
	logger   *slog.Logger
	cfg      Config
	ready    atomic.Bool
}

// New returns a worker with defaults filled in.
func New(pool *pgxpool.Pool, analyzer Analyzer, logger *slog.Logger, cfg Config) *Worker {
	return &Worker{
		pool:     pool,
		queries:  db.New(pool),
		analyzer: analyzer,
		logger:   logger,
		cfg:      applyDefaults(cfg),
	}
}

func applyDefaults(cfg Config) Config {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.SweepInterval == 0 {
		cfg.SweepInterval = 10 * time.Minute
	}
	if cfg.JobTimeout == 0 {
		cfg.JobTimeout = 10 * time.Minute
	}
	if cfg.BaseBackoff == 0 {
		cfg.BaseBackoff = 30 * time.Second
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Minute
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5
	}
	if cfg.BackoffDelay == 0 {
		cfg.BackoffDelay = 5 * time.Second
	}
	return cfg
}

// IsReady reports whether the worker holds a LISTEN connection.
func (w *Worker) IsReady() bool {
	return w.ready.Load()
}

// Run processes the queue, reconnecting after a lost connection, until ctx is
// cancelled.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting analysis worker",
		"poll_interval", w.cfg.PollInterval,
		"sweep_interval", w.cfg.SweepInterval,
		"job_timeout", w.cfg.JobTimeout,
	)

	for {
		err := w.runWithConnection(ctx)
		w.ready.Store(false)
		if ctx.Err() != nil {
			return fmt.Errorf("worker stopped: %w", ctx.Err())
		}
		w.logger.Error("connection lost, reconnecting", "error", err, "delay", w.cfg.BackoffDelay)
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
		return fmt.Errorf("acquire connection for LISTEN: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+listenChannel); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}
	w.ready.Store(true)

	var lastSweep time.Time
	for {
		if time.Since(lastSweep) >= w.cfg.SweepInterval {
			w.sweep(ctx)
			lastSweep = time.Now()
		}
		w.drain(ctx)

		if err := w.waitForNotification(ctx, conn); err != nil {
			return err
		}
	}
}

func (w *Worker) sweep(ctx context.Context) {
	enqueued, err := w.Sweep(ctx)
	if err != nil {
		w.logger.Error("sweep failed", "error", err)
		return
	}
	if enqueued > 0 {
		w.logger.Info("enqueued versions without an analysis", "count", enqueued)
	}
}

// Sweep enqueues every submitted version that has no analysis yet,
// and reports how many it enqueued.
func (w *Worker) Sweep(ctx context.Context) (int64, error) {
	enqueued, err := w.queries.AnalysisEnqueueMissing(ctx)
	if err != nil {
		return 0, fmt.Errorf("enqueueing versions without an analysis: %w", err)
	}
	return enqueued, nil
}

// drain processes rows until none is claimable.
func (w *Worker) drain(ctx context.Context) {
	for ctx.Err() == nil {
		found, err := w.ProcessNext(ctx)
		if err != nil {
			w.logger.Error("failed to process analysis", "error", err)
			return
		}
		if !found {
			return
		}
	}
}

func (w *Worker) waitForNotification(ctx context.Context, conn *pgxpool.Conn) error {
	waitCtx, cancel := context.WithTimeout(ctx, w.cfg.PollInterval)
	defer cancel()

	_, err := conn.Conn().WaitForNotification(waitCtx)
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return fmt.Errorf("shutdown requested: %w", ctx.Err())
	case errors.Is(err, context.DeadlineExceeded):
		// IsClosed alone misses a silently dead TCP connection.
		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		defer pingCancel()
		if err := conn.Conn().Ping(pingCtx); err != nil {
			return fmt.Errorf("connection health check failed: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("waiting for notification: %w", err)
	}
}

// ProcessNext claims and analyzes one row. It reports whether there was one.
func (w *Worker) ProcessNext(ctx context.Context) (bool, error) {
	row, err := w.queries.AnalysisClaim(ctx, db.AnalysisClaimParams{
		Lease: interval(w.cfg.JobTimeout + time.Minute),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claiming analysis: %w", err)
	}

	logger := w.logger.With("analysis_id", row.ID, "plugin_definition_id", row.PluginDefinitionID, "image", row.Image)

	if row.Retries > w.cfg.MaxRetries {
		// Claimed back from expired leases this often: the image kills or
		// stalls the worker every time.
		return true, w.fail(&row, logger, &analyze.PermanentError{
			Reason: fmt.Sprintf("The analysis did not finish after %d attempts", row.Retries),
		})
	}

	jobCtx, cancel := context.WithTimeout(ctx, w.cfg.JobTimeout)
	started := time.Now()
	result, err := w.analyzer.Analyze(jobCtx, row.Image)
	cancel()

	switch {
	case err == nil:
		logger.Info("analyzed image",
			"duration", time.Since(started), "packages", len(result.Packages), "files", len(result.Files))
		return true, w.complete(&row, result)
	case ctx.Err() != nil:
		// Shutting down: hand the row back rather than count an attempt.
		finalizeCtx, finalizeCancel := context.WithTimeout(context.Background(), finalizeTimeout)
		defer finalizeCancel()
		if err := w.queries.AnalysisRelease(finalizeCtx, db.AnalysisReleaseParams{ID: row.ID, LeasedUntil: row.RetryAfter}); err != nil {
			return true, fmt.Errorf("releasing analysis: %w", err)
		}
		return true, nil
	}

	var permanent *analyze.PermanentError
	if errors.As(err, &permanent) || row.Retries >= w.cfg.MaxRetries {
		return true, w.fail(&row, logger, err)
	}

	logger.Warn("analysis failed, retrying", "error", err, "retries", row.Retries)
	finalizeCtx, finalizeCancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer finalizeCancel()
	if err := w.queries.AnalysisRetry(finalizeCtx, db.AnalysisRetryParams{
		ID:          row.ID,
		LeasedUntil: row.RetryAfter,
		StatusInfo:  fmt.Sprintf("Attempt %d failed: %v", row.Retries+1, err),
		BaseBackoff: interval(w.cfg.BaseBackoff),
		MaxBackoff:  interval(w.cfg.MaxBackoff),
	}); err != nil {
		return true, fmt.Errorf("scheduling retry: %w", err)
	}
	return true, nil
}

// complete records the result of the attempt that holds row's lease. When the
// lease ran out and another attempt claimed the row, it changes nothing.
func (w *Worker) complete(row *db.AnalysisClaimRow, result *analyze.Result) error {
	platforms, err := json.Marshal(nonNil(result.Platforms))
	if err != nil {
		return fmt.Errorf("encoding platforms: %w", err)
	}
	packages, err := json.Marshal(nonNil(result.Packages))
	if err != nil {
		return fmt.Errorf("encoding packages: %w", err)
	}
	files, err := json.Marshal(nonNil(result.Files))
	if err != nil {
		return fmt.Errorf("encoding files: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer cancel()
	if err := w.queries.AnalysisComplete(ctx, db.AnalysisCompleteParams{
		ID:             row.ID,
		LeasedUntil:    row.RetryAfter,
		ImageDigest:    result.ImageDigest,
		Platform:       result.Platform,
		Platforms:      platforms,
		Packages:       packages,
		Files:          files,
		FilesTruncated: result.FilesTruncated,
		Sbom:           result.SBOM,
	}); err != nil {
		return fmt.Errorf("recording analysis: %w", err)
	}
	return nil
}

// fail records a failure for good. The reviewer reads status_info, so a
// permanent error contributes its reason, not its wrapped detail.
func (w *Worker) fail(row *db.AnalysisClaimRow, logger *slog.Logger, err error) error {
	info := err.Error()
	var platforms []string
	if permanent, ok := errors.AsType[*analyze.PermanentError](err); ok {
		info = permanent.Reason
		platforms = permanent.Platforms
	}
	logger.Warn("analysis failed", "error", err)

	encoded, encErr := json.Marshal(nonNil(platforms))
	if encErr != nil {
		return fmt.Errorf("encoding platforms: %w", encErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer cancel()
	if err := w.queries.AnalysisFail(ctx, db.AnalysisFailParams{
		ID: row.ID, LeasedUntil: row.RetryAfter, StatusInfo: info, Platforms: encoded,
	}); err != nil {
		return fmt.Errorf("recording failure: %w", err)
	}
	return nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func interval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}
