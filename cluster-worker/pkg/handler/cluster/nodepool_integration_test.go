package cluster_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// --- Group 3: Database schema trigger and constraint tests ---

func TestNodePoolInsertFiresTrigger(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "trigger-insert")

	// Clear the cluster outbox row created by inserting the cluster
	_, err := db.adminPool.Exec(t.Context(),
		`DELETE FROM tenant.cluster_outbox WHERE cluster_id = $1`, clusterID)
	require.NoError(t, err)

	// Insert a node pool — trigger should create outbox row with node_pool_id
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	var outboxNodePoolID uuid.UUID
	var source, event string
	err = db.adminPool.QueryRow(t.Context(),
		`SELECT node_pool_id, source, event FROM tenant.cluster_outbox
		 WHERE node_pool_id = $1
		 ORDER BY id DESC LIMIT 1`, nodePoolID).Scan(&outboxNodePoolID, &source, &event)
	require.NoError(t, err)
	require.Equal(t, nodePoolID, outboxNodePoolID)
	require.Equal(t, "trigger", source)
	require.Equal(t, "created", event)
}

func TestNodePoolSoftDeleteFiresTrigger(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "trigger-delete")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	// Soft-delete the node pool
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET deleted = now() WHERE id = $1`, nodePoolID)
	require.NoError(t, err)

	var event string
	err = db.adminPool.QueryRow(t.Context(),
		`SELECT event FROM tenant.cluster_outbox
		 WHERE node_pool_id = $1
		 ORDER BY id DESC LIMIT 1`, nodePoolID).Scan(&event)
	require.NoError(t, err)
	require.Equal(t, "deleted", event)
}

func TestSourceNodePoolRejectedByConstraint(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "constraint-source")

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
		 VALUES ($1, 'updated', 'node_pool')`, clusterID)
	require.Error(t, err, "source='node_pool' should be rejected by constraint")
}

func TestBothClusterIDAndNodePoolIDRejectedByConstraint(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "constraint-fk")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, node_pool_id, event, source)
		 VALUES ($1, $2, 'updated', 'trigger')`, clusterID, nodePoolID)
	require.Error(t, err, "both cluster_id and node_pool_id should be rejected by constraint")
}

// insertClusterOutboxRow adds a cluster outbox row in the given status, due at retryAfter.
func insertClusterOutboxRow(t *testing.T, db *testDB, clusterID uuid.UUID, status, retryAfter string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, retry_after)
		 VALUES ($1, 'updated', 'trigger', $2, $3::timestamptz)
		 RETURNING id`, clusterID, status, retryAfter).Scan(&id)
	require.NoError(t, err)
	return id
}

// outboxRowDue reports whether a cluster outbox row is due for processing now.
func outboxRowDue(t *testing.T, db *testDB, id uuid.UUID) bool {
	t.Helper()

	var due bool
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT retry_after IS NULL OR retry_after <= now() FROM tenant.cluster_outbox WHERE id = $1`, id).Scan(&due)
	require.NoError(t, err)
	return due
}

func TestNodePoolInsertWakesRunnableClusterRows(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "wake-runnable")
	markOutboxCompleted(t, db, clusterID)

	parked := insertClusterOutboxRow(t, db, clusterID, "pending", "infinity")
	backingOff := insertClusterOutboxRow(t, db, clusterID, "retrying", "infinity")
	failed := insertClusterOutboxRow(t, db, clusterID, "failed", "infinity")
	require.False(t, outboxRowDue(t, db, parked))
	require.False(t, outboxRowDue(t, db, backingOff))

	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	assert.True(t, outboxRowDue(t, db, parked), "a parked row is woken")
	assert.True(t, outboxRowDue(t, db, backingOff), "a row in retry backoff is woken")
	assert.False(t, outboxRowDue(t, db, failed), "a failed row is left alone")
}

func TestNodePoolInsertLeavesOtherClustersParked(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "wake-this")
	otherID := insertCluster(t, db, acmeCorpOrgID, "wake-other")
	markOutboxCompleted(t, db, otherID)
	otherParked := insertClusterOutboxRow(t, db, otherID, "pending", "infinity")

	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	assert.False(t, outboxRowDue(t, db, otherParked))
}

func TestNodePoolUpdateAndDeleteDoNotWake(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "wake-only-insert")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)
	markOutboxCompleted(t, db, clusterID)
	parked := insertClusterOutboxRow(t, db, clusterID, "pending", "infinity")

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET autoscale_max = 6 WHERE id = $1`, nodePoolID)
	require.NoError(t, err)
	assert.False(t, outboxRowDue(t, db, parked), "a resize does not wake")

	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET deleted = now() WHERE id = $1`, nodePoolID)
	require.NoError(t, err)
	assert.False(t, outboxRowDue(t, db, parked), "a soft delete does not wake")
}

// A row parked until the cluster's first pool would wait forever once the
// cluster is deleted; the delete wakes it.
func TestClusterDeleteWakesParkedRows(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "wake-on-delete")
	markOutboxCompleted(t, db, clusterID)
	parked := insertClusterOutboxRow(t, db, clusterID, "pending", "infinity")

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET deleted = now() WHERE id = $1`, clusterID)
	require.NoError(t, err)

	assert.True(t, outboxRowDue(t, db, parked))
}

func TestSyncWaitingForPreconditionEventAccepted(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "waiting-event")

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_events (cluster_id, event_type, message) VALUES ($1, $2, 'Creating cluster: waiting for a node pool')`,
		clusterID, string(dbconst.ClusterEventEventType_SyncWaitingForPrecondition))
	require.NoError(t, err)
}

// --- Group 4: Query tests ---

func TestNodePoolGetClusterID_SoftDeleted(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "get-cluster-id")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	// Soft-delete the node pool
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET deleted = now() WHERE id = $1`, nodePoolID)
	require.NoError(t, err)

	// Query should still return the cluster_id
	var resolvedClusterID uuid.UUID
	err = db.workerPool.QueryRow(t.Context(),
		`SELECT cluster_id FROM tenant.node_pools WHERE id = $1`, nodePoolID).Scan(&resolvedClusterID)
	require.NoError(t, err)
	require.Equal(t, clusterID, resolvedClusterID)
}

func TestClusterHasEverBeenSynced_TrueWithCompletedRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "ever-synced-true")

	// Mark outbox as completed
	markOutboxCompleted(t, db, clusterID)

	// Insert a new retrying row (simulating a subsequent change)
	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status)
		 VALUES ($1, 'updated', 'trigger', 'retrying')`, clusterID)
	require.NoError(t, err)

	// Should still return true because a completed row exists
	var synced bool
	err = db.workerPool.QueryRow(t.Context(),
		`SELECT EXISTS (
			SELECT 1 FROM tenant.cluster_outbox
			WHERE cluster_id = $1 AND status = 'completed'
		)`, clusterID).Scan(&synced)
	require.NoError(t, err)
	require.True(t, synced)
}

func TestClusterHasEverBeenSynced_FalseWhenNeverCompleted(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "ever-synced-false")

	// Don't mark anything as completed — cluster has pending outbox row from insert trigger

	var synced bool
	err := db.workerPool.QueryRow(t.Context(),
		`SELECT EXISTS (
			SELECT 1 FROM tenant.cluster_outbox
			WHERE cluster_id = $1 AND status = 'completed'
		)`, clusterID).Scan(&synced)
	require.NoError(t, err)
	require.False(t, synced)
}

// --- Group 7: Handler EntityNodePool tests ---

func TestSyncNodePoolResolvesToCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "np-sync")
	// Mark cluster as ever-synced so precondition passes
	markOutboxCompleted(t, db, clusterID)

	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Updated, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), nodePoolID, sc)
	require.NoError(t, err)

	// Shoot was applied with the node pool
	require.Len(t, mock.ApplyCalls, 1)
	require.Equal(t, clusterID, mock.ApplyCalls[0].ID)
	require.Len(t, mock.ApplyCalls[0].NodePools, 1)
	require.Equal(t, "workers", mock.ApplyCalls[0].NodePools[0].Name)
}

func TestSyncNodePool_ClusterNotSyncedLeavesItToTheClusterRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// The cluster's own created row is still pending.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "np-not-synced")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	require.NoError(t, h.Sync(t.Context(), nodePoolID, sc))

	assert.Empty(t, mock.ApplyCalls, "the cluster row creates the Shoot, not the pool row")
	var clusterRows int
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox WHERE cluster_id = $1`, clusterID).Scan(&clusterRows))
	assert.Equal(t, 1, clusterRows, "a pending cluster row is already queued; no second one")
}

// A cluster row another worker holds may have read the pools before this one
// was inserted, so it does not count as a sync still to come.
func TestSyncNodePool_ClusterRowClaimedElsewhereQueuesANewSync(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "np-cluster-claimed")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	// Another worker's claim: the cluster's created row is locked.
	claim, err := db.adminPool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = claim.Rollback(context.Background()) }()
	_, err = claim.Exec(t.Context(),
		`SELECT id FROM tenant.cluster_outbox WHERE cluster_id = $1 FOR NO KEY UPDATE`, clusterID)
	require.NoError(t, err)

	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	require.NoError(t, h.Sync(t.Context(), nodePoolID, sc))

	assert.Empty(t, mock.ApplyCalls)
	var pending int
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox WHERE cluster_id = $1 AND status = 'pending'`, clusterID).Scan(&pending))
	assert.Equal(t, 2, pending, "a second sync is queued, since the claimed one may miss the pool")
}

func TestSyncNodePool_ClusterRowFailedQueuesANewSync(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "np-cluster-failed")
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'failed', failed = now() WHERE cluster_id = $1`, clusterID)
	require.NoError(t, err)
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	require.NoError(t, h.Sync(t.Context(), nodePoolID, sc))

	assert.Empty(t, mock.ApplyCalls)
	var pending int
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox WHERE cluster_id = $1 AND status = 'pending'`, clusterID).Scan(&pending))
	assert.Equal(t, 1, pending, "a fresh cluster sync is queued")
}
func TestSyncClusterPreconditionError_NamespaceNotReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	mock.SimulateAsyncNamespace = true
	mock.NamespaceReadyDelay = 24 * time.Hour // never ready during test
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-precond")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.Error(t, err)

	var precondErr *handler.PreconditionError
	require.ErrorAs(t, err, &precondErr)
	require.Contains(t, precondErr.Reason, "project namespace not ready")
	assert.Equal(t, 5*time.Second, precondErr.FirstRetryAfter, "a new project is rechecked quickly once")

	// No ApplyShoot calls
	require.Empty(t, mock.ApplyCalls)
}

func TestSyncNodePoolNotFound(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	nonExistentID := uuid.New()
	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), nonExistentID, sc)

	// Precondition check handles ErrNoRows gracefully (returns nil, letting
	// syncNodePool handle the not-found case). syncNodePool also returns nil.
	require.NoError(t, err)

	// No Gardener calls
	require.Empty(t, mock.ApplyCalls)
}

// --- Group 9: Precondition declaration tests ---

func TestSyncNodePool_AfterClusterSyncedSyncsTheCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "np-after-synced")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)
	markOutboxCompleted(t, db, clusterID)

	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	require.NoError(t, h.Sync(t.Context(), nodePoolID, sc))
	require.Len(t, mock.ApplyCalls, 1)
}

func TestSyncCluster_WithoutNodePoolParks(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "park-no-pool")

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	var precondErr *handler.PreconditionError
	require.ErrorAs(t, err, &precondErr)
	assert.True(t, precondErr.Park)
	assert.Equal(t, cluster.MsgWaitingForNodePool, precondErr.Reason)

	assert.Empty(t, mock.ApplyCalls, "no Shoot without a worker")
	assert.Empty(t, mock.EnsureProjectCalls, "no Gardener project either")
}

func TestSyncCluster_DeletedWithoutNodePoolDoesNotPark(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "park-deleted")

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Deleted, Source: dbconst.ClusterOutboxSource_Trigger}
	require.NoError(t, h.Sync(t.Context(), clusterID, sc))
}

func TestSyncCluster_SyncedBeforeDoesNotPark(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// A cluster that reached Gardener and has no pool (only a legacy cluster
	// can) is not parked: its Shoot exists.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "park-synced")
	markOutboxCompleted(t, db, clusterID)

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Updated, Source: dbconst.ClusterOutboxSource_Trigger}
	require.NoError(t, h.Sync(t.Context(), clusterID, sc))
	require.Len(t, mock.ApplyCalls, 1)
}

// --- Group 17: End-to-end vertical slice tests ---

func TestNodePoolInsertTriggersOutboxAndHandlerSyncs(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Create a cluster and mark it as synced
	clusterID := insertCluster(t, db, acmeCorpOrgID, "e2e-np-sync")
	markOutboxCompleted(t, db, clusterID)

	// Insert node pool — trigger creates outbox row with node_pool_id
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "gpu", "n1-highmem-8", 0, 3)

	// Verify outbox row exists with correct fields
	var source, event string
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT source, event FROM tenant.cluster_outbox
		 WHERE node_pool_id = $1 ORDER BY id DESC LIMIT 1`, nodePoolID).Scan(&source, &event)
	require.NoError(t, err)
	require.Equal(t, "trigger", source)
	require.Equal(t, "created", event)

	// Handler processes the node pool outbox row → syncs cluster to Gardener
	sc := handler.SyncContext{
		EntityType: handler.EntityNodePool,
		Event:      dbconst.ClusterOutboxEvent(event),
		Source:     dbconst.ClusterOutboxSource(source),
	}
	err = h.Sync(t.Context(), nodePoolID, sc)
	require.NoError(t, err)

	// Verify Gardener received the full cluster spec with the new node pool
	require.Len(t, mock.ApplyCalls, 1)
	require.Equal(t, clusterID, mock.ApplyCalls[0].ID)
	require.Len(t, mock.ApplyCalls[0].NodePools, 1)
	require.Equal(t, "gpu", mock.ApplyCalls[0].NodePools[0].Name)
}
