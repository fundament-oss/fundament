package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
)

// clusterID is seeded by db/testdata/001_0101-content.sql. The outbox row needs
// a valid FK target; the entity type itself is irrelevant here because dispatch
// is stubbed out.
const seededClusterID = "019b4000-2000-7000-8000-000000000001"

// TestProcessOneItem_DispatchAbortsTx verifies that when dispatch fails in a way
// that aborts the transaction (SQLSTATE 25P02), the worker still records the
// retry on a fresh pool connection and surfaces the real dispatch error rather
// than the masking "current transaction is aborted" error.
func TestProcessOneItem_DispatchAbortsTx(t *testing.T) {
	pool := createTestDB(t)

	// Seeding test data fires the sync triggers, which populate authz.outbox.
	// Clear it so our row is the one GetAndLockNextOutboxRow (ORDER BY created)
	// picks up.
	_, err := pool.Exec(t.Context(), `DELETE FROM authz.outbox`)
	require.NoError(t, err)

	var itemID uuid.UUID
	err = pool.QueryRow(t.Context(),
		`INSERT INTO authz.outbox (cluster_id) VALUES ($1) RETURNING id`,
		seededClusterID,
	).Scan(&itemID)
	require.NoError(t, err)

	w := &Worker{
		pool:    pool,
		queries: db.New(pool),
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:     applyDefaults(Config{}),
	}

	// Simulate a handler whose SQL statement fails mid-transaction: the failing
	// statement poisons tx, after which any further query on tx returns 25P02.
	wantErr := errors.New("boom: handler sql failed")
	w.dispatch = func(ctx context.Context, tx pgx.Tx, _ *db.Queries, _ *db.GetAndLockNextOutboxRowRow) error {
		_, execErr := tx.Exec(ctx, "SELECT 1 / 0")
		require.Error(t, execErr, "the poisoning statement must fail")
		return wantErr
	}

	found, err := w.processOneItem(t.Context())

	require.True(t, found)
	// The real dispatch error must surface, not a masked "transaction is aborted".
	require.ErrorIs(t, err, wantErr)

	// The retry bookkeeping must have committed via a fresh connection.
	var (
		status     string
		retries    int32
		statusInfo *string
		retryAfter *time.Time
	)
	err = pool.QueryRow(t.Context(),
		`SELECT status, retries, status_info, retry_after FROM authz.outbox WHERE id = $1`,
		itemID,
	).Scan(&status, &retries, &statusInfo, &retryAfter)
	require.NoError(t, err)

	assert.Equal(t, "retrying", status)
	assert.Equal(t, int32(1), retries)
	require.NotNil(t, statusInfo)
	assert.Contains(t, *statusInfo, "boom")
	assert.NotNil(t, retryAfter, "retry backoff must be scheduled")
}
