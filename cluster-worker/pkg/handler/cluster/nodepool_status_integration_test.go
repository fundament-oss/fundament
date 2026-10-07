package cluster_test

import (
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

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Equal(t, "waiting", status)
	assert.Equal(t, enrNomach, message)
	extraStatus, _ := getNodePoolStatus(t, db, clusterID, "extra")
	assert.Equal(t, "progressing", extraStatus)

	assertEventExists(t, db, clusterID, "nodepool_waiting")
	assertEventExists(t, db, clusterID, "status_unhealthy")

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
	insertNodePool(t, db, clusterID, "workers", "local-medium", 2, 3)
	markOutboxCompleted(t, db, clusterID)
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

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Empty(t, status)
	assert.Empty(t, message)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_waiting"))
}
