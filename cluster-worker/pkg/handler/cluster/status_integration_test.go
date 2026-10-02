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
	err = h.CheckStatus(t.Context())
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

	err := h.CheckStatus(t.Context())
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

	err = h.CheckStatus(t.Context())
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

	err = h.CheckStatus(t.Context())
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

	err := h.CheckStatus(t.Context())
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

	err := h.CheckStatus(t.Context())
	require.NoError(t, err)

	message, health := getShootState(t, db, clusterID)
	assert.Equal(t, gardener.MsgShootReady, message)
	assert.Equal(t, "healthy", health)
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_healthy"))
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_ready"))
	assert.Equal(t, readyRowsBefore, countReadyOutboxRows(t, db, clusterID))
}

func TestCheckStatusReadyPollLanes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		health     string
		age        time.Duration
		wantPolled bool
	}{
		{name: "healthy, checked recently", health: "healthy", age: time.Minute, wantPolled: false},
		{name: "healthy, past the ready interval", health: "healthy", age: 6 * time.Minute, wantPolled: true},
		{name: "unhealthy, past 30 seconds", health: "unhealthy", age: time.Minute, wantPolled: true},
		{name: "unhealthy, checked just now", health: "unhealthy", age: 10 * time.Second, wantPolled: false},
		{name: "health unknown, past 30 seconds", health: "", age: time.Minute, wantPolled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := createTestDB(t)
			mock := newMock(t)
			h := newTestHandler(t, db, mock)

			clusterID := readyCluster(t, db, "status-lanes")
			setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, tt.health, tt.age)
			mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
				Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationReconcile, Healthy: true,
			})

			err := h.CheckStatus(t.Context())
			require.NoError(t, err)

			assert.Equal(t, tt.wantPolled, mock.StatusCallsFor(clusterID) > 0)
		})
	}
}

func TestCheckStatusOrdersNonReadyFirst(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandlerWithConfig(t, db, mock, cluster.Config{
		StatusBatchSize:     1,
		StatusReadyInterval: 5 * time.Minute,
		MaxRetries:          10,
	})

	// The ready cluster is far older, so plain staleness order would pick it.
	readyID := readyCluster(t, db, "status-order-ready")
	setShootState(t, db, readyID, "ready", gardener.MsgShootReady, "healthy", time.Hour)
	newID := readyCluster(t, db, "status-order-new")
	setShootState(t, db, newID, "progressing", "Create: Waiting", "", time.Minute)

	err := h.CheckStatus(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 1, mock.StatusCallsFor(newID))
	assert.Equal(t, 0, mock.StatusCallsFor(readyID))
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

	err := h.CheckStatus(t.Context())
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
		Status: gardener.StatusProgressing, Message: "Reconcile: Syncing", Operation: gardener.OperationReconcile,
	})

	err := h.CheckStatus(t.Context())
	require.NoError(t, err)

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "ready", *status)
	message, health := getShootState(t, db, clusterID)
	assert.Equal(t, gardener.MsgShootReady, message)
	assert.Equal(t, "healthy", health)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_progressing"))
	assert.Equal(t, readyRowsBefore, countReadyOutboxRows(t, db, clusterID))
	assert.Equal(t, 1, mock.StatusCallsFor(clusterID), "the row was polled and its timestamp moved")
}
