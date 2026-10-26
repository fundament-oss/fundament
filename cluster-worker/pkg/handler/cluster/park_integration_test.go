package cluster_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/outbox"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// clusterOutboxRow returns the id of a cluster's only outbox row.
func clusterOutboxRow(t *testing.T, db *testDB, clusterID uuid.UUID) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT id FROM tenant.cluster_outbox WHERE cluster_id = $1`, clusterID).Scan(&id)
	require.NoError(t, err)
	return id
}

// ignoreSync completes the status rows a sync emits, which other handlers
// take in the running worker.
type ignoreSync struct{}

func (ignoreSync) Sync(context.Context, uuid.UUID, handler.SyncContext) error { return nil }

// newParkWorker runs the outbox worker for a test, with the cluster handler
// registered for cluster and node pool rows.
func newParkWorker(t *testing.T, db *testDB, h *cluster.Handler) {
	t.Helper()

	registry := handler.NewRegistry()
	registry.RegisterSync(handler.EntityCluster, h)
	registry.RegisterSync(handler.EntityNodePool, h)
	registry.RegisterSyncForEvent(handler.EntityCluster, dbconst.ClusterOutboxEvent_Ready, ignoreSync{})
	registry.RegisterSyncForEvent(handler.EntityCluster, dbconst.ClusterOutboxEvent_ApiAvailable, ignoreSync{})
	worker := outbox.New(db.workerPool, registry, slog.New(slog.NewTextHandler(io.Discard, nil)), outbox.Config{
		PollInterval:             100 * time.Millisecond,
		BaseBackoff:              100 * time.Millisecond,
		MaxBackoff:               time.Second,
		MaxRetries:               15,
		BackoffDelay:             100 * time.Millisecond,
		PreconditionDelay:        30 * time.Second,
		MaxPreconditionDeferrals: 100,
	})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() { _ = worker.Run(ctx) }()
}

// The wake touches parked rows only, so a pool insert never waits for a row a
// worker holds while it syncs, and a row in backoff keeps its backoff.
func TestNodePoolInsertWakesOnlyParkedRows(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "wake-parked-only")
	markOutboxCompleted(t, db, clusterID)
	parked := insertClusterOutboxRow(t, db, clusterID, "pending", "infinity")
	backingOff := insertClusterOutboxRow(t, db, clusterID, "retrying", "infinity")
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET retry_after = now() + interval '1 hour' WHERE id = $1`, backingOff)
	require.NoError(t, err)

	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	assert.True(t, outboxRowDue(t, db, parked), "the parked row is woken")
	assert.False(t, outboxRowDue(t, db, backingOff), "a row in backoff keeps its backoff")
}

// A pool insert holds its cluster's row until it commits: what a park
// serializes on.
func TestNodePoolInsertHoldsTheClusterRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "insert-holds-cluster")

	tx, err := db.adminPool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(t.Context(),
		`INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
		 VALUES ($1, 'workers', 'n1-standard-4', 1, 5)`, clusterID)
	require.NoError(t, err)

	_, err = db.workerPool.Exec(t.Context(),
		`SELECT id FROM tenant.clusters WHERE id = $1 FOR NO KEY UPDATE NOWAIT`, clusterID)
	require.Error(t, err, "the cluster row is held until the pool commits")
	require.NoError(t, tx.Commit(t.Context()))
	_, err = db.workerPool.Exec(t.Context(),
		`SELECT id FROM tenant.clusters WHERE id = $1 FOR NO KEY UPDATE NOWAIT`, clusterID)
	require.NoError(t, err)
}

// The worker reads zero pools, then parks. A pool whose transaction is open
// meanwhile holds the cluster row; the park waits a little for it, then
// defers the row instead of parking it. Once the pool commits, the deferred
// row runs and syncs the cluster with its pool: never parked, never lost.
func TestOutboxWorker_PoolTransactionOpenDuringParkDefersThenSyncs(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "pool-during-park")
	id := clusterOutboxRow(t, db, clusterID)
	tx, err := db.adminPool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(t.Context(),
		`INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
		 VALUES ($1, 'workers', 'n1-standard-4', 1, 5)`, clusterID)
	require.NoError(t, err)

	newParkWorker(t, db, h)
	require.Eventually(t, func() bool {
		var deferrals int
		require.NoError(t, db.adminPool.QueryRow(t.Context(),
			`SELECT deferrals FROM tenant.cluster_outbox WHERE id = $1`, id).Scan(&deferrals))
		return deferrals > 0
	}, 10*time.Second, 50*time.Millisecond, "the park gives up on the lock and defers the row")
	assert.Zero(t, countEvents(t, db, clusterID, string(dbconst.ClusterEventEventType_SyncWaitingForPrecondition)),
		"not parked")
	assert.Empty(t, mock.ApplyCallsFor(clusterID))

	require.NoError(t, tx.Commit(t.Context()))
	require.Eventually(t, func() bool {
		return len(mock.ApplyCallsFor(clusterID)) > 0
	}, 10*time.Second, 50*time.Millisecond, "the deferred row runs with the pool")
	assert.Zero(t, countEvents(t, db, clusterID, string(dbconst.ClusterEventEventType_SyncWaitingForPrecondition)))
	require.Len(t, mock.ApplyCallsFor(clusterID)[0].NodePools, 1)
}

// End to end through the outbox worker: a cluster without a pool parks and
// says why; its first pool gets it its Shoot, with that pool as the worker.
func TestOutboxWorker_ClusterWaitsForItsFirstNodePool(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	newParkWorker(t, db, h)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "waits-for-pool")
	id := clusterOutboxRow(t, db, clusterID)

	require.Eventually(t, func() bool {
		return countEvents(t, db, clusterID, string(dbconst.ClusterEventEventType_SyncWaitingForPrecondition)) == 1
	}, 10*time.Second, 50*time.Millisecond, "the park records why the cluster waits")
	assert.False(t, outboxRowDue(t, db, id), "the row is parked")
	assert.Empty(t, mock.ApplyCallsFor(clusterID), "no Shoot without a worker")

	// A second row of the waiting cluster parks too, without a second event,
	// and neither park counts as a deferral.
	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source) VALUES ($1, 'updated', 'trigger')`, clusterID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var parked int
		require.NoError(t, db.adminPool.QueryRow(t.Context(),
			`SELECT count(*) FROM tenant.cluster_outbox WHERE cluster_id = $1 AND retry_after = 'infinity'`,
			clusterID).Scan(&parked))
		return parked == 2
	}, 10*time.Second, 50*time.Millisecond, "both rows park")
	var deferrals int
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT coalesce(sum(deferrals), 0) FROM tenant.cluster_outbox WHERE cluster_id = $1`, clusterID).Scan(&deferrals))
	assert.Zero(t, deferrals, "a park is not a deferral")
	assert.Equal(t, 1, countEvents(t, db, clusterID, string(dbconst.ClusterEventEventType_SyncWaitingForPrecondition)),
		"the wait is recorded once, not once per row")

	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	require.Eventually(t, func() bool {
		return len(mock.ApplyCallsFor(clusterID)) > 0
	}, 10*time.Second, 50*time.Millisecond, "the first pool gets the cluster its Shoot")
	first := mock.ApplyCallsFor(clusterID)[0]
	require.Len(t, first.NodePools, 1)
	assert.Equal(t, "workers", first.NodePools[0].Name)
	assert.Equal(t, 1, countEvents(t, db, clusterID, string(dbconst.ClusterEventEventType_SyncWaitingForPrecondition)),
		"the wait is recorded once")
}

// A cluster deleted while it waits for its first pool: the delete wakes the
// parked row, which runs the delete path and completes, so no row is left
// pending.
func TestOutboxWorker_DeletedWaitingClusterLeavesNoParkedRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	newParkWorker(t, db, h)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "deleted-while-waiting")
	id := clusterOutboxRow(t, db, clusterID)
	require.Eventually(t, func() bool {
		return countEvents(t, db, clusterID, string(dbconst.ClusterEventEventType_SyncWaitingForPrecondition)) == 1
	}, 10*time.Second, 50*time.Millisecond, "the cluster waits for a pool")

	_, err := db.adminPool.Exec(t.Context(), `UPDATE tenant.clusters SET deleted = now() WHERE id = $1`, clusterID)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var pending int
		require.NoError(t, db.adminPool.QueryRow(t.Context(),
			`SELECT count(*) FROM tenant.cluster_outbox WHERE cluster_id = $1 AND status IN ('pending', 'retrying')`,
			clusterID).Scan(&pending))
		return pending == 0
	}, 10*time.Second, 50*time.Millisecond, "every row of the deleted cluster completes, the parked one included")

	var status string
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT status FROM tenant.cluster_outbox WHERE id = $1`, id).Scan(&status))
	assert.Equal(t, "completed", status)
	var notCompleted int
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox WHERE cluster_id = $1 AND status <> 'completed'`,
		clusterID).Scan(&notCompleted))
	assert.Zero(t, notCompleted, "the delete row completed too, it did not fail")
	assert.Empty(t, mock.ApplyCallsFor(clusterID), "no Shoot was ever created")

	// There was no Shoot to wait for, so the cluster is confirmed gone at once
	// and stops counting against the organization's cluster quota (see
	// organization_clusters_in_use), instead of at the next status sweep.
	var shootStatus string
	var countsAgainstQuota bool
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT shoot_status, deleted IS NULL OR shoot_status IS DISTINCT FROM 'deleted'
		 FROM tenant.clusters WHERE id = $1`, clusterID).Scan(&shootStatus, &countsAgainstQuota))
	assert.Equal(t, "deleted", shootStatus)
	assert.False(t, countsAgainstQuota, "the quota slot is free")
	assertEventExists(t, db, clusterID, string(dbconst.ClusterEventEventType_StatusDeleted))
}

// A row another worker completed in between is left alone by the park.
func TestOutboxPark_CompletedRowIsNotParked(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "park-completed")
	id := clusterOutboxRow(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	parked, err := gen.New(db.workerPool).OutboxPark(t.Context(), gen.OutboxParkParams{
		ID:         id,
		StatusInfo: pgtype.Text{String: cluster.MsgWaitingForNodePool, Valid: true},
	})
	require.NoError(t, err)
	assert.Zero(t, parked)
	var retryAfter pgtype.Timestamptz
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT retry_after FROM tenant.cluster_outbox WHERE id = $1`, id).Scan(&retryAfter))
	assert.False(t, retryAfter.Valid, "not parked")
}
