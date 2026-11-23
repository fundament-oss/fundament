package cluster_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// outboxRowStatus returns an outbox row's status.
func outboxRowStatus(t *testing.T, db *testDB, id uuid.UUID) string {
	t.Helper()

	var status string
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT status FROM tenant.cluster_outbox WHERE id = $1`, id).Scan(&status)
	require.NoError(t, err)
	return status
}

// The outbox worker releases a row's lock before it records a failure, so
// another worker can claim and complete the row in between. Every failure write
// carries the status it expects, so the late write must leave the completed row
// alone instead of overwriting its outcome.
func TestOutboxLateFailureWriteLeavesCompletedRowAlone(t *testing.T) {
	t.Parallel()

	lateWrites := []struct {
		name  string
		write func(t *testing.T, queries *dbgen.Queries, id uuid.UUID)
	}{
		{name: "mark failed", write: func(t *testing.T, queries *dbgen.Queries, id uuid.UUID) {
			t.Helper()
			rows, err := queries.OutboxMarkFailed(t.Context(), dbgen.OutboxMarkFailedParams{
				ID:         id,
				StatusInfo: pgtype.Text{String: "apply shoot: gardener unreachable", Valid: true},
			})
			require.NoError(t, err)
			assert.Zero(t, rows, "a completed row is not marked failed")
		}},
		{name: "mark retry", write: func(t *testing.T, queries *dbgen.Queries, id uuid.UUID) {
			t.Helper()
			_, err := queries.OutboxMarkRetry(t.Context(), dbgen.OutboxMarkRetryParams{
				ID:           id,
				BaseInterval: pgtype.Interval{Microseconds: time.Second.Microseconds(), Valid: true},
				MaxBackoff:   pgtype.Interval{Microseconds: time.Minute.Microseconds(), Valid: true},
				StatusInfo:   pgtype.Text{String: "apply shoot: gardener unreachable", Valid: true},
			})
			require.ErrorIs(t, err, pgx.ErrNoRows, "a completed row is not retried")
		}},
		{name: "defer", write: func(t *testing.T, queries *dbgen.Queries, id uuid.UUID) {
			t.Helper()
			_, err := queries.OutboxDeferWithoutRetry(t.Context(), dbgen.OutboxDeferWithoutRetryParams{
				ID:         id,
				Delay:      pgtype.Interval{Microseconds: time.Second.Microseconds(), Valid: true},
				StatusInfo: pgtype.Text{String: "waiting for a node pool", Valid: true},
			})
			require.ErrorIs(t, err, pgx.ErrNoRows, "a completed row is not deferred")
		}},
	}

	for _, tc := range lateWrites {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := createTestDB(t)
			queries := dbgen.New(db.workerPool)
			clusterID := insertCluster(t, db, acmeCorpOrgID, "late-write")
			id := clusterOutboxRow(t, db, clusterID)

			// Worker A claimed the row, its handler failed, and it rolled back:
			// the row is pending and unlocked. Worker B claims and completes it.
			processed, err := queries.OutboxMarkProcessed(t.Context(), dbgen.OutboxMarkProcessedParams{ID: id})
			require.NoError(t, err)
			require.EqualValues(t, 1, processed)

			// Worker A now records its failure.
			tc.write(t, queries, id)

			assert.Equal(t, "completed", outboxRowStatus(t, db, id),
				"a row another worker completed stays completed")
		})
	}
}

// The status guard also refuses to complete a row that a failure already
// finalized.
func TestOutboxMarkProcessedLeavesFailedRowAlone(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "processed-after-failed")
	id := clusterOutboxRow(t, db, clusterID)

	rows, err := queries.OutboxMarkFailed(t.Context(), dbgen.OutboxMarkFailedParams{
		ID:         id,
		StatusInfo: pgtype.Text{String: "gardener rejected the shoot", Valid: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	processed, err := queries.OutboxMarkProcessed(t.Context(), dbgen.OutboxMarkProcessedParams{ID: id})
	require.NoError(t, err)
	require.Zero(t, processed, "a failed row is not completed")

	assert.Equal(t, "failed", outboxRowStatus(t, db, id),
		"a failed row is not completed by a late success write")
}
