package cluster_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// setShootStatusSince backdates when the stored status began, as if the check
// that wrote it ran then.
func setShootStatusSince(t *testing.T, db *testDB, clusterID uuid.UUID, age time.Duration) {
	t.Helper()

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET shoot_status_since = now() - make_interval(secs => $2) WHERE id = $1`,
		clusterID, age.Seconds(),
	)
	require.NoError(t, err)
}

// getShootStatusSince returns the cluster's shoot_status_since (zero for NULL).
func getShootStatusSince(t *testing.T, db *testDB, clusterID uuid.UUID) time.Time {
	t.Helper()

	var since *time.Time
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT shoot_status_since FROM tenant.clusters WHERE id = $1`, clusterID,
	).Scan(&since)
	require.NoError(t, err)
	if since == nil {
		return time.Time{}
	}
	return *since
}

// shoot_status_since is the start of the status, not of its message: a check
// that only rewrites the message leaves it, so a timeout measures how long the
// status lasted.
func TestShootStatusSinceMovesOnStatusChangeOnly(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-since")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "progressing", "Create: first task", "", 10*time.Minute)
	setShootStatusSince(t, db, clusterID, 10*time.Minute)
	before := getShootStatusSince(t, db, clusterID)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationCreate,
		Message: "Create: second task",
	})
	require.NoError(t, checkStatus(t, h))

	message, _ := getShootState(t, db, clusterID)
	require.Equal(t, "Create: second task", message)
	assert.WithinDuration(t, before, getShootStatusSince(t, db, clusterID), time.Second,
		"a message-only change must not move shoot_status_since")

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusReady, Operation: gardener.OperationCreate,
		Message: gardener.MsgShootReady, Healthy: true, APIServerAvailable: true,
	})
	require.NoError(t, checkStatus(t, h))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	require.Equal(t, "ready", *status)
	assert.WithinDuration(t, time.Now(), getShootStatusSince(t, db, clusterID), time.Minute,
		"a status change starts the status over")
}

// A Shoot that never appeared is not created by waiting: past the timeout the
// cluster reads error with the reason, and a later check that still finds
// nothing keeps that error — the Shoot did not disappear, so it is not lost.
func TestCheckStatusTimesOutANeverCreatedShoot(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-never")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "pending", gardener.MsgShootNotFound, "", 21*time.Minute)
	setShootStatusSince(t, db, clusterID, 21*time.Minute)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusPending, Message: gardener.MsgShootNotFound,
	})
	require.NoError(t, checkStatus(t, h))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "error", *status)
	message, _ := getShootState(t, db, clusterID)
	assert.Equal(t, "The Shoot did not appear in Gardener; the cluster's next change tries again.", message)
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_error"))

	require.NoError(t, checkStatus(t, h))
	status = getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "error", *status, "a timed-out cluster stays error, not lost")
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_lost"))
}

// An update that settled in the outbox long ago and is still rolling out has
// stalled: the check records a warning once and leaves the updating flag set,
// so a genuinely stuck update stays visible.
func TestCheckStatusWarnsOnAStalledUpdate(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-stalled")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", 2*time.Hour)
	setShootStatusSince(t, db, clusterID, 2*time.Hour)
	setShootUpdating(t, db, clusterID, true)

	// The update reached Gardener 45 minutes ago; its sync row is the latest.
	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, processed)
		 VALUES ($1, 'updated', 'trigger', 'completed', now() - interval '45 minutes')`, clusterID)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message: "Reconcile: Waiting until worker resource status is updated", APIServerAvailable: true,
	})
	require.NoError(t, checkStatus(t, h))

	var warning string
	err = db.adminPool.QueryRow(t.Context(),
		`SELECT message FROM tenant.cluster_events WHERE cluster_id = $1 AND event_type = 'status_warning'`,
		clusterID,
	).Scan(&warning)
	require.NoError(t, err)
	assert.Equal(t, "The update has not finished long after the change reached Gardener.", warning)

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "ready", *status)

	var updating bool
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT shoot_updating FROM tenant.clusters WHERE id = $1`, clusterID).Scan(&updating))
	assert.True(t, updating, "the updating flag must stay set")

	require.NoError(t, checkStatus(t, h))
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_warning"), "the warning is recorded once")
}

// A Shoot still deleting past the alert limit records one warning; the status
// stays deleting, because forcing deleted would release the cluster's quota
// while Gardener still has the Shoot.
func TestCheckDeletedClusterWarnsOnAStuckDeletion(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "tf-stuck-delete")
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "deleting", "Delete: in progress", "", 3*time.Hour)
	setShootStatusSince(t, db, clusterID, 3*time.Hour)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusDeleting, Operation: gardener.OperationDelete, Message: "Delete: in progress",
	})
	require.NoError(t, checkStatus(t, h))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "deleting", *status)
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_warning"))

	require.NoError(t, checkStatus(t, h))
	assert.Equal(t, 1, countEvents(t, db, clusterID, "status_warning"))
}
