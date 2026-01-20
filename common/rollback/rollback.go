// Package rollback provides a convenient helper function that rolls back a
// transaction or logs an error when it fails.
package rollback

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// Rollbacker is an interface that requires a Rollback method. This is
// implemented by pgx.Tx.
type Rollbacker interface {
	Rollback(ctx context.Context) error
}

// Rollback rolls back the transaction tx. Rollback will log an error if the
// transaction could not be rolled back and was not committed.
func Rollback(ctx context.Context, tx Rollbacker, logger *slog.Logger) {
	err := tx.Rollback(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrTxClosed) ||
			errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return
		}
		logger.Error("failed to rollback db transaction", "error", err)
	}
}
