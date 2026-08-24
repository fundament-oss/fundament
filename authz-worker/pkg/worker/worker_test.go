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

// TestProcessesRetryingRowOnPollTimeoutWithoutNotify verifies the worker drains
// due 'retrying' rows on its poll-interval timeout, not only when a NOTIFY
// arrives. The outbox_notify trigger fires AFTER INSERT only, so a row a prior
// worker instance left in 'retrying' emits no notification when its retry_after
// elapses. If the worker processed exclusively on NOTIFY, such a row would lag
// forever and trip organization-api's circuit breaker.
func TestProcessesRetryingRowOnPollTimeoutWithoutNotify(t *testing.T) {
	pool := createTestDB(t)

	// Start from an empty outbox so the worker's initial batch has nothing to do
	// and it parks in waitForNotification.
	_, err := pool.Exec(t.Context(), `DELETE FROM authz.outbox`)
	require.NoError(t, err)

	w := &Worker{
		pool:    pool,
		queries: db.New(pool),
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:     applyDefaults(Config{PollInterval: 50 * time.Millisecond}),
	}
	// Dispatch stub that succeeds, so a processed row is marked completed.
	w.dispatch = func(context.Context, pgx.Tx, *db.Queries, *db.GetAndLockNextOutboxRowRow) error {
		return nil
	}
	w.verify = func(context.Context) error { return nil }

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.runWithConnection(ctx) }()

	require.Eventually(t, w.IsReady, 2*time.Second, 10*time.Millisecond)

	// Insert the row as 'retrying' but not yet due (retry_after far in the
	// future). The AFTER INSERT notify wakes the worker, but GetAndLockNextOutboxRow
	// skips it (retry_after > now), so it is not processed yet.
	var itemID uuid.UUID
	err = pool.QueryRow(t.Context(),
		`INSERT INTO authz.outbox (cluster_id, status, retries, retry_after)
		 VALUES ($1, 'retrying', 1, now() + interval '1 hour') RETURNING id`,
		seededClusterID,
	).Scan(&itemID)
	require.NoError(t, err)

	// Let the insert-triggered batch run and find nothing due, then park again.
	time.Sleep(150 * time.Millisecond)

	// Make the row due via UPDATE — the outbox_notify trigger is INSERT-only, so
	// this fires NO notification. The row is now reachable only via the
	// poll-timeout code path.
	_, err = pool.Exec(t.Context(),
		`UPDATE authz.outbox SET retry_after = now() WHERE id = $1`, itemID)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var status string
		if err := pool.QueryRow(t.Context(),
			`SELECT status FROM authz.outbox WHERE id = $1`, itemID,
		).Scan(&status); err != nil {
			return false
		}
		return status == "completed"
	}, 3*time.Second, 20*time.Millisecond, "due retrying row must be processed on the poll timeout without a NOTIFY")

	cancel()
	<-done
}

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
		verify:  func(context.Context) error { return nil },
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

// TestHoldsOutboxWhenStoreUnavailable verifies the worker refuses to drain when
// the OpenFGA store check fails. OpenFGA accepts writes to a store that no
// longer exists, so a drain in that state marks rows processed against nothing
// and they are never replayed.
func TestHoldsOutboxWhenStoreUnavailable(t *testing.T) {
	pool := createTestDB(t)

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
		cfg:     applyDefaults(Config{PollInterval: 50 * time.Millisecond}),
		verify:  func(context.Context) error { return errStoreUnavailable },
	}
	// Fails the test if reached: a failed store check must stop the drain.
	w.dispatch = func(context.Context, pgx.Tx, *db.Queries, *db.GetAndLockNextOutboxRowRow) error {
		t.Error("dispatched an outbox row while the store was unavailable")

		return nil
	}

	err = w.runWithConnection(t.Context())
	require.ErrorIs(t, err, errStoreUnavailable)
	assert.False(t, w.IsReady(), "must not report ready without its store")

	var status string
	var processed *time.Time
	err = pool.QueryRow(t.Context(),
		`SELECT status, processed FROM authz.outbox WHERE id = $1`, itemID,
	).Scan(&status, &processed)
	require.NoError(t, err)
	assert.Equal(t, "pending", status, "row must be held, not consumed")
	assert.Nil(t, processed, "row must not be marked processed")
}
