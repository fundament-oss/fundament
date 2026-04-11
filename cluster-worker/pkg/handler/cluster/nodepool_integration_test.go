package cluster_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
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

func TestFanInTriggerPropagatesNodePoolStatus(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "fanin-np")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	// Mark the node pool outbox row as completed
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox
		 SET status = 'completed', processed = now()
		 WHERE id = (
		     SELECT id FROM tenant.cluster_outbox
		     WHERE node_pool_id = $1
		     ORDER BY id DESC
		     LIMIT 1
		 )`, nodePoolID)
	require.NoError(t, err)

	// Check cluster's outbox_status was propagated
	var outboxStatus string
	err = db.adminPool.QueryRow(t.Context(),
		`SELECT outbox_status FROM tenant.clusters WHERE id = $1`, clusterID).Scan(&outboxStatus)
	require.NoError(t, err)
	require.Equal(t, "completed", outboxStatus)
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

func TestSyncNodePoolPreconditionError_ClusterNotSynced(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "np-precond")
	// Do NOT mark cluster as ever-synced
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), nodePoolID, sc)
	require.Error(t, err)

	var precondErr *handler.PreconditionError
	require.ErrorAs(t, err, &precondErr)
	require.Contains(t, precondErr.Reason, "parent cluster not synced")

	// No Gardener calls
	require.Empty(t, mock.ApplyCalls)
}

func TestSyncClusterPreconditionError_NamespaceNotReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	mock.SimulateAsyncNamespace = true
	mock.NamespaceReadyDelay = 24 * time.Hour // never ready during test
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-precond")

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.Error(t, err)

	var precondErr *handler.PreconditionError
	require.ErrorAs(t, err, &precondErr)
	require.Contains(t, precondErr.Reason, "project namespace not ready")

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

func TestPreconditionAllowsNodePoolAfterClusterSynced(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "precond-allow")
	nodePoolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)

	sc := handler.SyncContext{EntityType: handler.EntityNodePool, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}

	// Before cluster is synced: precondition fails
	err := h.Sync(t.Context(), nodePoolID, sc)
	require.Error(t, err)
	var precondErr *handler.PreconditionError
	require.ErrorAs(t, err, &precondErr)

	// Mark cluster as synced
	markOutboxCompleted(t, db, clusterID)

	// After cluster is synced: precondition passes, sync succeeds
	err = h.Sync(t.Context(), nodePoolID, sc)
	require.NoError(t, err)
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
