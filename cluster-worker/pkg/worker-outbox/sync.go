package worker_outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/rollback"
)

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
