package cluster_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	dbgen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/kubename"
)

// Gardener's text names the generated worker name, never the raw pool name.
var enrNomach = `machine deployment "shoot--acme--tf-a-` + kubename.GenerateWorkerName("nomach") + `-z1" is waiting for machines to join`

func TestCheckStatusRecordsWaitingNodePool(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-a")
	insertNodePool(t, db, clusterID, "extra", "n1-standard-4", 2, 3)
	insertNodePool(t, db, clusterID, "nomach", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)
	setShootUpdating(t, db, clusterID, true)

	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
	})

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Equal(t, "waiting", status)
	assert.Equal(t, enrNomach, message)
	extraStatus, _ := getNodePoolStatus(t, db, clusterID, "extra")
	assert.Equal(t, "progressing", extraStatus)

	assertEventExists(t, db, clusterID, "nodepool_waiting")
	// No status_unhealthy event: during a fundament-pushed rollout health is
	// tracked without events (readyDuringUpdate) — the pool event carries it.
	assertNoEvent(t, db, clusterID, "status_unhealthy")

	// A second check with the same message must not duplicate the event.
	require.NoError(t, checkStatus(t, h))
	require.Equal(t, 2, mock.StatusCallsFor(clusterID), "the sweep must have re-checked the cluster")
	assert.Equal(t, 1, countEvents(t, db, clusterID, "nodepool_waiting"))
	status, message = getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Equal(t, "waiting", status)
	assert.Equal(t, enrNomach, message)

	// The cluster stays ready in the database; health degraded.
	gotStatus := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, gotStatus)
	assert.Equal(t, "ready", *gotStatus)
	_, health := getShootState(t, db, clusterID)
	assert.Equal(t, "unhealthy", health)
}

func TestCheckStatusRecordsNodePoolRecovery(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-recover")
	insertNodePool(t, db, clusterID, "nomach", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	// Pre-set the pool to waiting, as if a prior poll had observed trouble.
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET status = 'waiting', status_message = $1 WHERE cluster_id = $2 AND name = 'nomach'`,
		enrNomach, clusterID,
	)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Operation: gardener.OperationReconcile,
		Message: "Shoot is ready", Healthy: true,
	})

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Equal(t, "ready", status)
	assert.Empty(t, message)

	assert.Equal(t, 1, countEvents(t, db, clusterID, "nodepool_ready"))
	_, health := getShootState(t, db, clusterID)
	assert.Equal(t, "healthy", health)
}

func TestCheckStatusClassifiesExistingPoolsSilently(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-silent")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 2, 3)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Operation: gardener.OperationReconcile,
		Message: "Shoot is ready", Healthy: true,
	})

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, "ready", status)
	assert.Empty(t, message)

	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_ready"))
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_waiting"))
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_error"))
}

func TestCheckStatusSkipsDeletedPools(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-deleted-pool")
	insertNodePool(t, db, clusterID, "nomach", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET deleted = now() WHERE cluster_id = $1 AND name = 'nomach'`,
		clusterID,
	)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
	})

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Empty(t, status)
	assert.Empty(t, message)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_waiting"))
}

// A change is in flight while the cluster's own latest sync has not completed,
// or a node pool's latest row is queued, or failed for a pool still active. A
// cluster event that is not a sync (the ready fan-out) is not a change.
func TestClusterGetPushStateSyncPending(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	setPoolRow := func(poolID uuid.UUID, status string) {
		_, err := db.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = $2 WHERE node_pool_id = $1`, poolID, status)
		require.NoError(t, err)
	}
	addClusterRow := func(clusterID uuid.UUID, event, status string) {
		_, err := db.adminPool.Exec(t.Context(),
			`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status) VALUES ($1, $2, 'trigger', $3)`,
			clusterID, event, status)
		require.NoError(t, err)
	}
	deletePool := func(poolID uuid.UUID) {
		_, err := db.adminPool.Exec(t.Context(),
			`UPDATE tenant.node_pools SET deleted = now() WHERE id = $1`, poolID)
		require.NoError(t, err)
	}

	cases := []struct {
		name  string
		setup func(clusterID, poolID uuid.UUID)
		want  bool
	}{
		{name: "everything synced", setup: func(uuid.UUID, uuid.UUID) {}, want: false},
		{name: "a pool's create still queued", setup: func(_, pool uuid.UUID) { setPoolRow(pool, "pending") }, want: true},
		{name: "a pool's sync failed", setup: func(_, pool uuid.UUID) { setPoolRow(pool, "failed") }, want: true},
		{name: "a deleted pool's sync failed", setup: func(_, pool uuid.UUID) {
			deletePool(pool)
			setPoolRow(pool, "failed")
		}, want: false},
		{name: "the cluster's own update queued", setup: func(c, _ uuid.UUID) { addClusterRow(c, "updated", "pending") }, want: true},
		{name: "a ready fan-out row queued", setup: func(c, _ uuid.UUID) { addClusterRow(c, "ready", "pending") }, want: false},
	}
	for i, tc := range cases {
		clusterID := insertCluster(t, db, acmeCorpOrgID, fmt.Sprintf("push-%d", i))
		poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 2)
		markClusterSyncsCompleted(t, db, clusterID)
		tc.setup(clusterID, poolID)

		push, err := queries.ClusterGetPushState(t.Context(), dbgen.ClusterGetPushStateParams{ClusterID: clusterID})
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.want, push.SyncPending, tc.name)
	}
}
