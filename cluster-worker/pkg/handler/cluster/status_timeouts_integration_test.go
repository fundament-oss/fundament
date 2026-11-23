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

// A cluster whose status row predates shoot_status_since has no start time;
// the check falls back to when the status was last written, so a cluster that
// was already stuck at deploy time still reaches the timeout instead of
// waiting forever.
func TestCheckStatusTimesOutAShootStuckBeforeTheColumnExisted(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-never-old-row")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "pending", gardener.MsgShootNotFound, "", 21*time.Minute)
	require.True(t, getShootStatusSince(t, db, clusterID).IsZero(),
		"the row must have no shoot_status_since for this test")

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusPending, Message: gardener.MsgShootNotFound,
	})
	require.NoError(t, checkStatus(t, h))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "error", *status)
	message, _ := getShootState(t, db, clusterID)
	assert.Equal(t, "The Shoot did not appear in Gardener; the cluster's next change tries again.", message)
}

// A pending Shoot ages past the limit while a sync is still queued: that sync
// will apply the change and create the Shoot — the sync path runs this check
// just before that apply — so the check must not record a failure first.
func TestCheckStatusDoesNotTimeOutWhileASyncIsQueued(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-pending-queued")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "pending", gardener.MsgShootNotFound, "", 21*time.Minute)
	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status)
		 VALUES ($1, 'updated', 'trigger', 'pending')`, clusterID)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusPending, Message: gardener.MsgShootNotFound,
	})
	require.NoError(t, checkStatus(t, h))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	assert.Equal(t, "pending", *status)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_error"))
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

// A sync row that failed for good never delivered the change, so its finish is
// no measure of how long Gardener has had it: no stall warning follows from it.
func TestCheckStatusDoesNotMeasureAStalledUpdateFromAFailedRow(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-failed-row")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", 2*time.Hour)
	setShootStatusSince(t, db, clusterID, 2*time.Hour)
	setShootUpdating(t, db, clusterID, true)

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, failed)
		 VALUES ($1, 'updated', 'trigger', 'failed', now() - interval '45 minutes')`, clusterID)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message: "Reconcile: Waiting until worker resource status is updated", APIServerAvailable: true,
	})
	require.NoError(t, checkStatus(t, h))

	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_warning"),
		"an update whose sync failed never reached Gardener, so it cannot have stalled there")
}

// While the update's own sync row is still queued, the check does not measure
// it against the previous row's finish: the change may not have reached
// Gardener yet, and a cluster idle for longer than the limit would warn at once.
func TestCheckStatusDoesNotWarnWhileTheUpdateIsStillQueued(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-queued-update")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", 2*time.Hour)
	setShootStatusSince(t, db, clusterID, 2*time.Hour)
	setShootUpdating(t, db, clusterID, true)

	// The last settled sync is 45 minutes old, the update's own row is still
	// waiting to run.
	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, processed)
		 VALUES ($1, 'updated', 'trigger', 'completed', now() - interval '45 minutes')`, clusterID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status)
		 VALUES ($1, 'updated', 'trigger', 'pending')`, clusterID)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message: "Reconcile: Waiting until worker resource status is updated", APIServerAvailable: true,
	})
	require.NoError(t, checkStatus(t, h))

	assert.Equal(t, 0, countEvents(t, db, clusterID, "status_warning"),
		"an update whose sync row is still queued has not stalled")
}

// A node pool's sync applies the whole cluster to Gardener, so it can be the
// row that carried an update. A cluster whose latest sync is such a row, long
// finished, has stalled exactly like one whose own row settled.
func TestCheckStatusWarnsWhenOnlyThePoolsSyncSettled(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-stalled-pool")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", 2*time.Hour)
	setShootStatusSince(t, db, clusterID, 2*time.Hour)
	setShootUpdating(t, db, clusterID, true)

	// The pool's change is the cluster's latest sync; it reached Gardener 45
	// minutes ago.
	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (node_pool_id, event, source, status, processed)
		 VALUES ($1, 'updated', 'trigger', 'completed', now() - interval '45 minutes')`, poolID)
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
}

// Two warnings can apply to one cluster at once: Gardener erroring on a retry
// while the update it is retrying has stalled. Each is recorded once, and
// neither displaces the other, so a repeated check records nothing.
func TestCheckStatusRecordsEachWarningOnceWhenTwoSourcesApply(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-two-warnings")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", gardener.MsgShootReady, "healthy", 2*time.Hour)
	setShootStatusSince(t, db, clusterID, 2*time.Hour)
	setShootUpdating(t, db, clusterID, true)

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, processed)
		 VALUES ($1, 'updated', 'trigger', 'completed', now() - interval '45 minutes')`, clusterID)
	require.NoError(t, err)

	retrying := "etcd not ready. Operation will be retried."
	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusError, Operation: gardener.OperationReconcile, Retrying: true,
		Message: retrying, APIServerAvailable: true,
	})
	require.NoError(t, checkStatus(t, h))

	rows, err := db.adminPool.Query(t.Context(),
		`SELECT message FROM tenant.cluster_events WHERE cluster_id = $1 AND event_type = 'status_warning'`,
		clusterID)
	require.NoError(t, err)
	defer rows.Close()
	var messages []string
	for rows.Next() {
		var message string
		require.NoError(t, rows.Scan(&message))
		messages = append(messages, message)
	}
	require.NoError(t, rows.Err())
	assert.ElementsMatch(t, []string{
		retrying,
		"The update has not finished long after the change reached Gardener.",
	}, messages)

	require.NoError(t, checkStatus(t, h))
	assert.Equal(t, 2, countEvents(t, db, clusterID, "status_warning"),
		"a repeated check must not record either warning again")
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
