package cluster_test

import (
	"errors"
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

func TestCheckStatusTransitionCreatesEvent(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert cluster, sync it to create the shoot, then mark outbox completed.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "status-ready")
	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.NoError(t, err)
	markOutboxCompleted(t, db, clusterID)

	// Set shoot_status_updated far enough in the past to bypass the 30s throttle.
	setShootStatus(t, db, clusterID, "progressing")

	// Mock returns Ready for this cluster (instant mock — shoot is immediately ready).
	err = checkStatus(t, h)
	require.NoError(t, err)

	// DB should have shoot_status = ready.
	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	require.Equal(t, "ready", *status)

	// status_ready event should exist.
	assertEventExists(t, db, clusterID, "status_ready")
}

func TestCheckStatusDeletedConfirmedGone(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert a soft-deleted cluster and mark outbox completed.
	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "status-deleted")
	markOutboxCompleted(t, db, clusterID)

	// Set shoot_status_updated to bypass the throttle.
	// No shoot exists in mock — GetShootStatus will return {StatusPending, MsgShootNotFound}.
	setShootStatus(t, db, clusterID, "deleting")

	err := checkStatus(t, h)
	require.NoError(t, err)

	// DB should have shoot_status = deleted.
	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	require.Equal(t, "deleted", *status)

	// status_deleted event should exist.
	assertEventExists(t, db, clusterID, "status_deleted")
}

func TestCheckStatusErrorTransition(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert cluster, sync it, mark completed.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "status-error")
	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.NoError(t, err)
	markOutboxCompleted(t, db, clusterID)
	setShootStatus(t, db, clusterID, "progressing")

	// Override mock to return error status.
	mock.SetStatusOverride(clusterID, gardener.StatusError, "shoot reconciliation failed")

	err = checkStatus(t, h)
	require.NoError(t, err)

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	require.Equal(t, "error", *status)

	assertEventExists(t, db, clusterID, "status_error")
}

func TestCheckStatusSkipsProjectLookup(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "status-no-project")
	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.NoError(t, err)
	markOutboxCompleted(t, db, clusterID)
	setShootStatus(t, db, clusterID, "progressing")

	// A failing project lookup must not block the status poll.
	callsBefore := len(mock.EnsureProjectCalls)
	mock.EnsureProjectError = errors.New("garden unavailable")

	err = checkStatus(t, h)
	require.NoError(t, err)

	assert.Len(t, mock.EnsureProjectCalls, callsBefore)
	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "ready", *status)
}

func TestCheckStatusDeletedSkipsProjectLookup(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "status-deleted-no-project")
	markOutboxCompleted(t, db, clusterID)
	setShootStatus(t, db, clusterID, "deleting")

	// Confirming a deletion must neither need nor (re)create the org's project.
	mock.EnsureProjectError = errors.New("garden unavailable")

	err := checkStatus(t, h)
	require.NoError(t, err)

	assert.Empty(t, mock.EnsureProjectCalls)
	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "deleted", *status)
}

// readyCluster inserts a cluster whose outbox has completed, so the status
// query considers it synced.
func readyCluster(t *testing.T, db *testDB, name string) uuid.UUID {
	t.Helper()

	clusterID := insertCluster(t, db, acmeCorpOrgID, name)
	markOutboxCompleted(t, db, clusterID)
	return clusterID
}

// The #1257 regression: the first ready reading often has conditions still
// settling. The poller must come back and record the settled state.
func TestCheckStatusRefreshesUnhealthyReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "status-settles")
	setShootState(t, db, clusterID, "ready", gardener.MsgShootUnhealthy, "unhealthy", time.Minute)
	readyRowsBefore := countReadyOutboxRows(t, db, clusterID)
	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationCreate, Healthy: true,
	})

	err := checkStatus(t, h)
	require.NoError(t, err)

	message, health := getShootState(t, db, clusterID)
	assert.Equal(t, gardener.MsgShootReady, message)
	assert.Equal(t, "healthy", health)
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_healthy"))
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_ready"))
	assert.Equal(t, readyRowsBefore, countReadyOutboxRows(t, db, clusterID))
}

// The sweep checks every cluster that has reached Gardener, whatever its
// status and however recently it was checked, plus soft-deleted clusters
// whose Shoot is not confirmed gone.
func TestCheckStatusSweepsEveryCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	healthy := readyCluster(t, db, "sweep-healthy")
	setShootState(t, db, healthy, "ready", gardener.MsgShootReady, "healthy", time.Second)
	creating := readyCluster(t, db, "sweep-creating")
	setShootState(t, db, creating, "progressing", "Create: Waiting", "", time.Second)
	unsynced := insertCluster(t, db, acmeCorpOrgID, "sweep-unsynced")
	deleting := insertDeletedCluster(t, db, acmeCorpOrgID, "sweep-deleting")
	setShootState(t, db, deleting, "deleting", "Shoot is being deleted", "", time.Second)
	gone := insertDeletedCluster(t, db, acmeCorpOrgID, "sweep-gone")
	setShootState(t, db, gone, "deleted", "Shoot confirmed deleted", "", time.Second)

	require.NoError(t, checkStatus(t, h))

	assert.Equal(t, 1, mock.StatusCallsFor(healthy))
	assert.Equal(t, 1, mock.StatusCallsFor(creating))
	assert.Equal(t, 1, mock.StatusCallsFor(deleting))
	assert.Equal(t, 0, mock.StatusCallsFor(unsynced), "no Shoot yet")
	assert.Equal(t, 0, mock.StatusCallsFor(gone), "confirmed deleted")
}

// A check that would record nothing new writes nothing, so a routine
// reconcile's many Shoot updates cost reads only.
func TestCheckClusterUnchangedWritesNothing(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "status-unchanged")
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", time.Hour)
	before := getShootStatusUpdated(t, db, clusterID)
	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationReconcile, Healthy: true,
	})

	require.NoError(t, h.CheckCluster(t.Context(), clusterID))

	assert.Equal(t, 1, mock.StatusCallsFor(clusterID))
	assert.Equal(t, before, getShootStatusUpdated(t, db, clusterID))
}

// An error Gardener will retry (state Error or Aborted) is recorded as a
// warning, not as a cluster error.
func TestCheckStatusRetriedErrorIsWarning(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "status-retrying")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting", "", time.Minute)
	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusError, Message: "etcd not ready. Operation will be retried.", Operation: gardener.OperationCreate, Retrying: true,
	})

	err := checkStatus(t, h)
	require.NoError(t, err)

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "progressing", *status)
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_warning"))
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_error"))
}

// Gardener reconciles every running shoot periodically; that must not flip a
// ready cluster to progressing or re-run the ready fan-out.
func TestCheckStatusRoutineReconcileKeepsRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "status-reconcile")
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", 6*time.Minute)
	readyRowsBefore := countReadyOutboxRows(t, db, clusterID)
	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusProgressing, Message: "Reconcile: Syncing", Operation: gardener.OperationReconcile, Healthy: true,
	})

	err := checkStatus(t, h)
	require.NoError(t, err)

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "ready", *status)
	message, health := getShootState(t, db, clusterID)
	assert.Equal(t, gardener.MsgShootReady, message)
	assert.Equal(t, "healthy", health)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_progressing"))
	assert.Equal(t, readyRowsBefore, countReadyOutboxRows(t, db, clusterID))
	assert.Equal(t, 1, mock.StatusCallsFor(clusterID), "the cluster was checked")
}

// A status check for a cluster row that does not exist does nothing.
func TestCheckClusterMissingRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	missing := uuid.New()
	require.NoError(t, h.CheckCluster(t.Context(), missing))
	assert.Equal(t, 0, mock.StatusCallsFor(missing))
}

// A cluster that has not reached Gardener yet has no Shoot to check; the
// caller is told to try again.
func TestCheckClusterNotSyncedYet(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "status-unsynced")
	require.ErrorIs(t, h.CheckCluster(t.Context(), clusterID), cluster.ErrClusterNotSynced)
	assert.Equal(t, 0, mock.StatusCallsFor(clusterID))
	assert.Nil(t, getClusterShootStatus(t, db, clusterID))
}

// Two checks of the same transition racing on one row record it once: the
// guarded update lets one of them write, and the events and the ready outbox
// row are written in the same transaction as that update.
func TestCheckClusterConcurrentTransitionRecordedOnce(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "status-race")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting", "", time.Minute)
	readyRowsBefore := countReadyOutboxRows(t, db, clusterID)
	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationCreate, Healthy: true,
	})

	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- h.CheckCluster(t.Context(), clusterID) }()
	}
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)

	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_ready"))
	assert.Equal(t, readyRowsBefore+1, countReadyOutboxRows(t, db, clusterID))
}

// Gardener retries a failing task again and again, with progress in between
// that the watch sees; the same error is recorded as one warning.
func TestCheckClusterRetriedErrorRecordedOnce(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "status-retry-once")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting", "", time.Minute)

	const failure = `task "Deploying gardener-resource-manager" failed: token not yet generated. Operation will be retried.`
	report := func(status gardener.ShootStatusType, message string, retrying bool) {
		mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
			Status: status, Message: message, Operation: gardener.OperationCreate, Retrying: retrying,
		})
		require.NoError(t, h.CheckCluster(t.Context(), clusterID))
	}

	report(gardener.StatusError, failure, true)
	report(gardener.StatusProgressing, "Create: Deploying gardener-resource-manager", false)
	report(gardener.StatusError, failure, true)

	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_warning"))
}

// The same error in a later incident is a new warning: once the cluster
// reached ready, an earlier warning no longer counts as the one it repeats.
func TestCheckClusterRetriedErrorAfterReadyIsNewWarning(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "status-retry-again")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting", "", time.Minute)

	const failure = `task "Deploying gardener-resource-manager" failed: token not yet generated. Operation will be retried.`
	report := func(status gardener.ShootStatusType, message string, retrying, healthy bool) {
		mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
			Status: status, Message: message, Operation: gardener.OperationCreate, Retrying: retrying, Healthy: healthy,
		})
		require.NoError(t, h.CheckCluster(t.Context(), clusterID))
	}

	report(gardener.StatusError, failure, true, false)
	report(gardener.StatusReady, gardener.MsgShootReady, false, true)
	report(gardener.StatusError, failure, true, false)

	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_ready"))
	assert.Equal(t, 2, countEvents(t, db, clusterID, "status_warning"))
}
