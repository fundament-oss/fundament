package cluster_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

const enrNomach = `machine deployment "shoot--acme--tf-a-nomach-z1" is waiting for machines to join`

func TestCheckStatusRecordsWaitingNodePool(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-a")
	insertNodePool(t, db, clusterID, "extra", "local-medium", 2, 3)
	insertNodePool(t, db, clusterID, "nomach", "local-medium", 1, 1)
	markOutboxCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
	})

	require.NoError(t, h.CheckStatus(context.Background()))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Equal(t, "waiting", status)
	assert.Equal(t, enrNomach, message)
	extraStatus, _ := getNodePoolStatus(t, db, clusterID, "extra")
	assert.Equal(t, "progressing", extraStatus)

	assertEventExists(t, db, clusterID, "nodepool_waiting")
	assertEventExists(t, db, clusterID, "status_unhealthy")

	// Back-date shoot_status_updated so the cluster is genuinely re-polled
	// (otherwise the 30s fast-lane throttle skips it and the second
	// CheckStatus call would be a no-op, making the dedupe check below vacuous).
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "unhealthy", time.Minute)

	// A second poll with the same message must not duplicate the event.
	require.NoError(t, h.CheckStatus(context.Background()))
	require.Equal(t, 2, mock.StatusCallsFor(clusterID), "the cluster must have been genuinely re-polled, not skipped by the throttle")
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
	insertNodePool(t, db, clusterID, "nomach", "local-medium", 1, 1)
	markOutboxCompleted(t, db, clusterID)
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

	require.NoError(t, h.CheckStatus(context.Background()))

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
	insertNodePool(t, db, clusterID, "workers", "local-medium", 2, 3)
	markOutboxCompleted(t, db, clusterID)
	// Stale shoot_status_updated so the fast lane picks this up thanks to the
	// NULL-status pool clause, even though the cluster is ready and healthy.
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Operation: gardener.OperationReconcile,
		Message: "Shoot is ready", Healthy: true,
	})

	require.NoError(t, h.CheckStatus(context.Background()))

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
	insertNodePool(t, db, clusterID, "nomach", "local-medium", 1, 1)
	markOutboxCompleted(t, db, clusterID)
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

	require.NoError(t, h.CheckStatus(context.Background()))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Empty(t, status)
	assert.Empty(t, message)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_waiting"))
}

// A pool that has never been classified (NULL status) keeps a ready, healthy
// cluster on the fast lane; once CheckStatus classifies it ready, the cluster
// drops back to the slow lane.
func TestCheckStatusNodePoolKeepsClusterOnFastLane(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-fastlane")
	insertNodePool(t, db, clusterID, "workers", "local-medium", 2, 3)
	markOutboxCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Operation: gardener.OperationReconcile,
		Message: "Shoot is ready", Healthy: true,
	})

	require.NoError(t, h.CheckStatus(context.Background()))
	assert.Equal(t, 1, mock.StatusCallsFor(clusterID), "NULL-status pool should have put the cluster on the fast lane")

	status, _ := getNodePoolStatus(t, db, clusterID, "workers")
	require.Equal(t, "ready", status)

	// Back the shoot_status_updated off by a minute again (CheckStatus just set it to now()).
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	require.NoError(t, h.CheckStatus(context.Background()))
	assert.Equal(t, 1, mock.StatusCallsFor(clusterID), "pool is now ready: the cluster must drop off the fast lane")
}
