package cluster_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	"github.com/fundament-oss/fundament/common/dbconst"
)

func TestSyncCreateHappyPath(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-create")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 5)
	insertNodePool(t, db, clusterID, "gpu", "n1-highmem-8", 0, 3)

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.NoError(t, err)

	// Shoot was created in Gardener
	require.Len(t, mock.ApplyCalls, 1)
	applied := mock.ApplyCalls[0]
	require.Equal(t, clusterID, applied.ID)
	require.Equal(t, "sync-create", applied.Name)
	require.Equal(t, "eu-west-1", applied.Region)
	require.Equal(t, "1.31.1", applied.KubernetesVersion)

	// Node pools passed correctly
	require.Len(t, applied.NodePools, 2)
	require.Equal(t, gardener.NodePool{Name: "workers", MachineType: "n1-standard-4", AutoscaleMin: 1, AutoscaleMax: 5}, applied.NodePools[0])
	require.Equal(t, gardener.NodePool{Name: "gpu", MachineType: "n1-highmem-8", AutoscaleMin: 0, AutoscaleMax: 3}, applied.NodePools[1])

	// sync_succeeded event created
	assertEventExists(t, db, clusterID, "sync_succeeded")
}

func TestSyncDeleteHappyPath(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "sync-delete")

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Deleted, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.NoError(t, err)

	// DeleteByClusterID was called
	require.Len(t, mock.DeleteByClusterID, 1)
	require.Equal(t, clusterID, mock.DeleteByClusterID[0])

	// No ApplyShoot calls
	require.Empty(t, mock.ApplyCalls)

	// sync_succeeded event created
	assertEventExists(t, db, clusterID, "sync_succeeded")
}

func TestSyncClusterNotFound(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	nonExistentID := uuid.New()
	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), nonExistentID, sc)
	require.NoError(t, err, "Sync should return nil for non-existent cluster")

	// No Gardener calls
	require.Empty(t, mock.ApplyCalls)
	require.Empty(t, mock.DeleteByClusterID)
}

func TestSyncApplyShootError(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-error")
	mock.SetApplyError(gardener.ErrMockApplyFailed)

	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.Error(t, err)
	require.ErrorIs(t, err, gardener.ErrMockApplyFailed)

	// sync_failed event created
	assertEventExists(t, db, clusterID, "sync_failed")
	// No sync_succeeded
	assertNoEvent(t, db, clusterID, "sync_succeeded")
}

// syncedReadyCluster creates a cluster's Shoot in the mock and records it as
// a healthy ready cluster.
func syncedReadyCluster(t *testing.T, db *testDB, h *cluster.Handler, name string) uuid.UUID {
	t.Helper()

	clusterID := readyCluster(t, db, name)
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 3)
	created := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	require.NoError(t, h.Sync(t.Context(), clusterID, created))
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", time.Minute)
	return clusterID
}

var clusterUpdated = handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Updated, Source: dbconst.ClusterOutboxSource_Trigger}

// A change fundament pushes to a ready cluster marks it as updating until the
// reconcile it starts is done; it stays ready (usable) meanwhile, and the end
// of the update does not re-run the ready fan-out.
func TestSyncUpdateShowsReadyClusterAsUpdating(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := syncedReadyCluster(t, db, h, "sync-update")
	readyRowsBefore := countReadyOutboxRows(t, db, clusterID)

	insertNodePool(t, db, clusterID, "gpu", "n1-highmem-8", 0, 2)
	require.NoError(t, h.Sync(t.Context(), clusterID, clusterUpdated))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "ready", *status, "the cluster stays usable during the update")
	assert.True(t, getShootUpdating(t, db, clusterID))
	message, health := getShootState(t, db, clusterID)
	assert.Equal(t, gardener.MsgShootUpdatePending, message)
	assert.Equal(t, "healthy", health)
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_progressing"))

	// The instant mock finishes the reconcile right away.
	setShootState(t, db, clusterID, "ready", gardener.MsgShootUpdatePending, "healthy", time.Minute)
	require.NoError(t, checkStatus(t, h))

	status = getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "ready", *status)
	assert.False(t, getShootUpdating(t, db, clusterID))
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_ready"))
	assert.Equal(t, readyRowsBefore, countReadyOutboxRows(t, db, clusterID))
}

// Re-applying an unchanged cluster starts no reconcile in Gardener.
func TestSyncUnchangedUpdateKeepsReadyCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := syncedReadyCluster(t, db, h, "sync-noop")
	require.NoError(t, h.Sync(t.Context(), clusterID, clusterUpdated))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "ready", *status)
	assert.False(t, getShootUpdating(t, db, clusterID))
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_progressing"))
}

// A cluster that is not ready yet keeps the status the poll gave it.
func TestSyncUpdateLeavesClusterBeingCreated(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := syncedReadyCluster(t, db, h, "sync-creating")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting for etcd", "", time.Minute)

	insertNodePool(t, db, clusterID, "gpu", "n1-highmem-8", 0, 2)
	require.NoError(t, h.Sync(t.Context(), clusterID, clusterUpdated))

	message, health := getShootState(t, db, clusterID)
	assert.Equal(t, "Create: Waiting for etcd", message)
	assert.Empty(t, health)
	assert.False(t, getShootUpdating(t, db, clusterID))
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_progressing"))
}
