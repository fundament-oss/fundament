package cluster_test

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	dbgen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/kubename"
)

// Gardener's text names the generated worker name, never the raw pool name.
const enrTechnicalID = "shoot--acme--tf-a"

var enrNomach = `machine deployment "` + enrTechnicalID + `-` + kubename.GenerateWorkerName("nomach") + `-z1" is waiting for machines to join`

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

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
		TechnicalID:           enrTechnicalID,
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

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
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

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
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

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
		TechnicalID:           enrTechnicalID,
	})

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Empty(t, status)
	assert.Empty(t, message)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_waiting"))
}

// A later sync of a cluster that never reached Gardener changes nothing: it
// queues the cluster's own sync and completes, so the pool's failed row stays
// the only thing that says why its change never landed.
func TestCheckStatusKeepsAFailedPoolSyncBeforeTheShoot(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-failed-before-shoot")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	laterPool := insertNodePoolReturningID(t, db, clusterID, "later", "n1-standard-4", 1, 1)

	// Nothing of the cluster is queued anymore: its create gave up.
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'failed', failed = now()
		 WHERE tenant.cluster_outbox.cluster_id = $1 AND tenant.cluster_outbox.status <> 'completed'`, clusterID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'failed', status_info = 'quota exceeded', failed = now()
		 WHERE tenant.cluster_outbox.node_pool_id = $1`, poolID)
	require.NoError(t, err)
	// The later pool's sync completed while only queueing the cluster's own sync.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'completed', processed = now()
		 WHERE tenant.cluster_outbox.node_pool_id = $1`, laterPool)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusPending, Message: gardener.MsgShootNotFound,
	})
	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, "error", status)
	assert.Equal(t, "The last sync to Gardener failed: quota exceeded", message)
}

// A pool's failed sync row stops describing the pool once a later sync
// completes: that sync applied the whole cluster, so Gardener has the pool's
// change, and the current report decides again like for any other pool.
func TestCheckStatusForgetsAFailedPoolSyncSupersededByALaterSync(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-superseded-pool")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	// The pool's change ran out of retries; a later change to the cluster
	// completed and pushed the whole Shoot, this pool's change included.
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'failed', status_info = 'quota exceeded', failed = now()
		 WHERE tenant.cluster_outbox.node_pool_id = $1`, poolID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, processed)
		 VALUES ($1, 'updated', 'trigger', 'completed', now())`, clusterID)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusReady, Operation: gardener.OperationReconcile,
		Message: "Shoot is ready", Healthy: true,
	})
	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, "ready", status)
	assert.Empty(t, message)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_error"))
}

// Ids order the inserts, not the runs: the worker claims a row in retry
// backoff after higher ids. A sync inserted before the pool's row that
// processed after the failure was recorded applied the cluster all the same,
// so it supersedes the failed row.
func TestCheckStatusForgetsAFailedPoolSyncProcessedByAnOlderRow(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-superseded-by-processed")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "Shoot is ready", "healthy", time.Minute)

	// The pool's change ran out of retries. A row inserted before it, still
	// waiting out its backoff, ran afterwards and pushed the whole Shoot.
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'failed', status_info = 'quota exceeded', failed = now() - interval '1 minute'
		 WHERE tenant.cluster_outbox.node_pool_id = $1`, poolID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (id, node_pool_id, event, source, status, processed)
		 VALUES ('018f0000-0000-7000-8000-000000000001', $1, 'updated', 'trigger', 'completed', now())`, poolID)
	require.NoError(t, err)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusReady, Operation: gardener.OperationReconcile,
		Message: "Shoot is ready", Healthy: true,
	})
	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, "ready", status)
	assert.Empty(t, message)
	assert.Equal(t, 0, countEvents(t, db, clusterID, "nodepool_error"))
}

// A change is in flight while the cluster's own latest sync is queued or
// retrying, or a node pool's latest row is. A cluster event that is not a sync
// (the ready fan-out) is not a change.
//
// Blocked is the cluster's own latest sync row failing for good with no pool
// sync that came after it and can still apply it, later by insert or by having
// run after the failure: nothing retries it, so the pools say so instead of
// waiting. A pool's failed row is not blocked at the cluster level — the pool
// list carries its message, and a later sync may still succeed; a later pool
// row that failed itself does not lift the block, since nothing applied the
// cluster's change.
func TestClusterGetPushState(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	setPoolRow := func(poolID uuid.UUID, status string) {
		_, err := db.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = $2, status_info = 'sync failed' WHERE node_pool_id = $1`, poolID, status)
		require.NoError(t, err)
	}
	addClusterRow := func(clusterID uuid.UUID, event, status string) {
		_, err := db.adminPool.Exec(t.Context(),
			`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, status_info)
			 VALUES ($1, $2, 'trigger', $3, 'sync failed')`,
			clusterID, event, status)
		require.NoError(t, err)
	}
	deletePool := func(poolID uuid.UUID) {
		_, err := db.adminPool.Exec(t.Context(),
			`UPDATE tenant.node_pools SET deleted = now() WHERE id = $1`, poolID)
		require.NoError(t, err)
	}

	cases := []struct {
		name string
		// neverSynced skips the completed sync rows every other case starts
		// with, so the cluster never reached Gardener.
		neverSynced    bool
		setup          func(clusterID, poolID uuid.UUID)
		wantInFlight   bool
		wantQueued     bool
		wantBlocked    bool
		wantMessageSet bool
	}{
		{name: "everything synced", setup: func(uuid.UUID, uuid.UUID) {}},
		{name: "a pool's create still queued", setup: func(_, pool uuid.UUID) { setPoolRow(pool, "pending") }, wantInFlight: true, wantQueued: true},
		// A deleted pool's sync applies the whole cluster, so a queued one means
		// a Shoot is still to come: not in flight for the pools that remain,
		// which a pool removal does not shape, but queued all the same.
		{name: "a deleted pool's sync still queued", setup: func(_, pool uuid.UUID) {
			deletePool(pool)
			setPoolRow(pool, "pending")
		}, wantQueued: true},
		{name: "a pool's sync failed", setup: func(_, pool uuid.UUID) { setPoolRow(pool, "failed") }},
		{name: "a deleted pool's sync failed", setup: func(_, pool uuid.UUID) {
			deletePool(pool)
			setPoolRow(pool, "failed")
		}},
		{name: "the cluster's own update queued", setup: func(c, _ uuid.UUID) { addClusterRow(c, "updated", "pending") }, wantInFlight: true, wantQueued: true},
		// A row in retry backoff applies the cluster when it runs, whatever a
		// newer row did in between.
		{name: "an older cluster row still queued", setup: func(c, _ uuid.UUID) {
			addClusterRow(c, "updated", "retrying")
			addClusterRow(c, "updated", "completed")
		}, wantQueued: true},
		{name: "the cluster's own update failed", setup: func(c, _ uuid.UUID) { addClusterRow(c, "updated", "failed") }, wantBlocked: true, wantMessageSet: true},
		{name: "the cluster's own update failed and a later pool sync failed", setup: func(c, _ uuid.UUID) {
			addClusterRow(c, "updated", "failed")
			laterPool := insertNodePoolReturningID(t, db, c, "later", "n1-standard-4", 1, 2)
			setPoolRow(laterPool, "failed")
		}, wantBlocked: true, wantMessageSet: true},
		// A deleted pool's sync applies the whole cluster, failed change and
		// all, so it clears the block like any other pool's sync.
		{name: "a failed cluster row superseded by a deleted pool's sync", setup: func(c, pool uuid.UUID) {
			addClusterRow(c, "updated", "failed")
			deletePool(pool)
			_, err := db.adminPool.Exec(t.Context(),
				`UPDATE tenant.cluster_outbox SET status = 'completed', processed = now()
				 WHERE tenant.cluster_outbox.node_pool_id = $1`, pool)
			require.NoError(t, err)
		}},
		// A pool's sync of a cluster that never reached Gardener changes
		// nothing: it queues the cluster's own sync and completes. It must not
		// hide the cluster row's failure.
		{name: "a failed cluster row not superseded by a pool sync before the Shoot", neverSynced: true, setup: func(c, pool uuid.UUID) {
			setPoolRow(pool, "completed")
			addClusterRow(c, "updated", "failed")
			// The cluster's own create row failed for good too: nothing of the
			// cluster is queued anymore.
			_, err := db.adminPool.Exec(t.Context(),
				`UPDATE tenant.cluster_outbox SET status = 'failed', failed = now()
				 WHERE tenant.cluster_outbox.cluster_id = $1 AND tenant.cluster_outbox.status = 'pending'`, c)
			require.NoError(t, err)
			laterPool := insertNodePoolReturningID(t, db, c, "later", "n1-standard-4", 1, 2)
			setPoolRow(laterPool, "completed")
		}, wantBlocked: true, wantMessageSet: true},
		{name: "a ready fan-out row queued", setup: func(c, _ uuid.UUID) { addClusterRow(c, "ready", "pending") }},
		{name: "a failed cluster row superseded by a later pool sync", setup: func(c, _ uuid.UUID) {
			addClusterRow(c, "updated", "failed")
			insertNodePoolReturningID(t, db, c, "later", "n1-standard-4", 1, 2)
			_, err := db.adminPool.Exec(t.Context(),
				`UPDATE tenant.cluster_outbox SET status = 'completed', processed = now()
				 WHERE tenant.cluster_outbox.node_pool_id IN (
				     SELECT tenant.node_pools.id FROM tenant.node_pools WHERE tenant.node_pools.cluster_id = $1)
				   AND tenant.cluster_outbox.status = 'pending'`, c)
			require.NoError(t, err)
		}},
		// Ids order the inserts, not the runs: this pool's row was inserted
		// before the cluster's failed one, but ran after it and applied the
		// cluster's change.
		{name: "a failed cluster row superseded by a pool sync processed after it", setup: func(c, _ uuid.UUID) {
			addClusterRow(c, "updated", "failed")
			_, err := db.adminPool.Exec(t.Context(),
				`UPDATE tenant.cluster_outbox SET failed = now() - interval '10 minutes'
				 WHERE tenant.cluster_outbox.cluster_id = $1 AND tenant.cluster_outbox.status = 'failed'`, c)
			require.NoError(t, err)
			_, err = db.adminPool.Exec(t.Context(),
				`UPDATE tenant.cluster_outbox SET processed = now()
				 WHERE tenant.cluster_outbox.node_pool_id IN (
				     SELECT tenant.node_pools.id FROM tenant.node_pools WHERE tenant.node_pools.cluster_id = $1)`, c)
			require.NoError(t, err)
		}},
	}
	for i, tc := range cases {
		clusterID := insertCluster(t, db, acmeCorpOrgID, fmt.Sprintf("push-%d", i))
		poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 2)
		if !tc.neverSynced {
			markClusterSyncsCompleted(t, db, clusterID)
		}
		tc.setup(clusterID, poolID)

		push, err := queries.ClusterGetPushState(t.Context(), dbgen.ClusterGetPushStateParams{ClusterID: clusterID})
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.wantInFlight, push.SyncInFlight, tc.name)
		assert.Equal(t, tc.wantQueued, push.SyncQueued, tc.name)
		assert.Equal(t, tc.wantBlocked, push.SyncBlocked, tc.name)
		assert.Equal(t, tc.wantMessageSet, push.SyncBlockedMessage.Valid, tc.name)
	}
}

// Issue 1383: on metal-stack, machine names carry no zone suffix. A cluster
// being created with one pool metal-api has no machines for reports that pool
// waiting, with its machine type in plain words; the other pool is not named.
func TestCheckStatusRecordsWaitingMetalPool(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "metal")
	insertNodePool(t, db, clusterID, "small", "n1-standard-4", 2, 2)
	insertNodePool(t, db, clusterID, "medium", "n1-highmem-8", 2, 2)
	markClusterSyncsCompleted(t, db, clusterID)

	technicalID := "shoot--systemd819--enricotef4f"
	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationCreate,
		Message: "Create: Waiting until shoot worker nodes have been reconciled",
		EveryNodeReadyMessage: `Worker extension (` + technicalID + `/enricotef4f) reports failing health check: machine "` +
			technicalID + `-` + kubename.GenerateWorkerName("medium") + `-68c79-bnnvm" failed: ` +
			`[POST /v1/machine/allocate][422] allocateMachine default "{\"statuscode\":422,\"message\":\"no machine available\"}"`,
		TechnicalID: technicalID,
	})

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "medium")
	assert.Equal(t, "waiting", status)
	assert.Equal(t, "No free n1-highmem-8 machine is available.", message)
	smallStatus, smallMessage := getNodePoolStatus(t, db, clusterID, "small")
	assert.Equal(t, "progressing", smallStatus)
	assert.Empty(t, smallMessage)
	assert.Equal(t, 1, countEvents(t, db, clusterID, "nodepool_waiting"))
}

// The poc case of issue 1383: a create that hangs on one pool. The pool whose
// nodes joined is ready while Gardener still reconciles, the pool Gardener
// names waits, and both pools' node counts are stored for the console.
func TestCheckStatusSettlesPoolsByTheirOwnNodes(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)
	nodes := shoot.NewMockShootAccess(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))
	h.SetPoolNodeReader(nodes)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-own")
	insertNodePool(t, db, clusterID, "small", "n1-standard-4", 2, 2)
	insertNodePool(t, db, clusterID, "nomach", "n1-standard-4", 2, 2)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "progressing", "Create: Waiting until shoot worker nodes have been reconciled", "", time.Minute)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationCreate,
		Message:            "Create: Waiting until shoot worker nodes have been reconciled",
		APIServerAvailable: true, EveryNodeReadyMessage: enrNomach,
		TechnicalID: enrTechnicalID,
	})
	joined := time.Now()
	nodes.SetNodePools(clusterID, map[string][]shoot.PoolNode{
		kubename.GenerateWorkerName("small"): {{Created: joined, Ready: true}, {Created: joined, Ready: true}},
	})

	require.NoError(t, checkStatus(t, h))

	status, _ := getNodePoolStatus(t, db, clusterID, "small")
	assert.Equal(t, "ready", status)
	status, message := getNodePoolStatus(t, db, clusterID, "nomach")
	assert.Equal(t, "waiting", status)
	assert.Equal(t, enrNomach, message)
	assertEventExists(t, db, clusterID, "nodepool_waiting")

	n, ready, counted := getNodePoolCount(t, db, clusterID, "small")
	require.True(t, counted)
	assert.Equal(t, int32(2), n)
	assert.Equal(t, int32(2), ready)
	n, ready, counted = getNodePoolCount(t, db, clusterID, "nomach")
	require.True(t, counted)
	assert.Zero(t, n)
	assert.Zero(t, ready)

	// While a pool waits, the check asks to run again within a minute: the
	// nodes come up without Gardener writing the Shoot. The same counts are
	// not written again.
	updated := getNodePoolCountUpdated(t, db, clusterID, "small")
	retryAfter, err := h.CheckClusterRetryAfter(t.Context(), clusterID)
	require.NoError(t, err)
	assert.Positive(t, retryAfter)
	assert.LessOrEqual(t, retryAfter, time.Minute)
	assert.Equal(t, updated, getNodePoolCountUpdated(t, db, clusterID, "small"))
	assert.Equal(t, 1, countEvents(t, db, clusterID, "nodepool_waiting"))
}

// A pool replaced by one of the same name (a machine type changes that way)
// has the same worker name, so its predecessor's nodes carry its label. They
// are counted for the console but do not make the new pool ready.
func TestCheckStatusIgnoresPredecessorNodes(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)
	nodes := shoot.NewMockShootAccess(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))
	h.SetPoolNodeReader(nodes)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-replace")
	insertNodePool(t, db, clusterID, "workers", "n1-highmem-8", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "ready", "", "", time.Minute)
	setShootUpdating(t, db, clusterID, true)

	// Gardener rolls the change out; the old machine type's node stands.
	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message: "Reconcile: Waiting until shoot worker nodes have been reconciled", APIServerAvailable: true,
		TechnicalID: enrTechnicalID,
	})
	nodes.SetNodePools(clusterID, map[string][]shoot.PoolNode{
		kubename.GenerateWorkerName("workers"): {{Created: time.Now().Add(-time.Hour), Ready: true}},
	})

	require.NoError(t, checkStatus(t, h))

	status, _ := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, "progressing", status)
	assert.Zero(t, countEvents(t, db, clusterID, "nodepool_ready"))
	n, ready, counted := getNodePoolCount(t, db, clusterID, "workers")
	require.True(t, counted)
	assert.Equal(t, int32(1), n)
	assert.Equal(t, int32(1), ready)
}

// A failed read of the shoot's nodes changes nothing: the pools are
// classified from Gardener's report alone and no count is stored.
func TestCheckStatusIgnoresFailedNodeRead(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)
	nodes := shoot.NewMockShootAccess(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))
	nodes.ListNodePoolsError = fmt.Errorf("forbidden")
	h.SetPoolNodeReader(nodes)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-noread")
	insertNodePool(t, db, clusterID, "small", "n1-standard-4", 1, 1)
	markClusterSyncsCompleted(t, db, clusterID)
	setShootState(t, db, clusterID, "progressing", "Create: Waiting until shoot worker nodes have been reconciled", "", time.Minute)

	mock.SetShootStatusOverride(clusterID, &gardener.StatusOverride{
		Status: gardener.StatusProgressing, Operation: gardener.OperationCreate,
		Message: "Create: Waiting until shoot worker nodes have been reconciled", APIServerAvailable: true,
	})

	require.NoError(t, checkStatus(t, h))

	status, _ := getNodePoolStatus(t, db, clusterID, "small")
	assert.Equal(t, "progressing", status)
	_, _, counted := getNodePoolCount(t, db, clusterID, "small")
	assert.False(t, counted)
}

// A create that failed for good leaves the cluster without a Shoot. The sweep
// still reaches it, and the pools report why they were never built instead of
// staying blank behind a gate that never opens.
func TestCheckStatusRecordsAFailedCreateOnItsPools(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-failed-create")
	insertNodePool(t, db, clusterID, "workers", "n1-standard-4", 1, 1)
	markClusterSyncsFailed(t, db, clusterID, "invalid machine type")

	require.NoError(t, checkStatus(t, h))

	status, message := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, "error", status)
	assert.Equal(t, "The last sync to Gardener failed: invalid machine type", message)
	assertEventExists(t, db, clusterID, "nodepool_error")
	assert.Equal(t, 1, mock.StatusCallsFor(clusterID), "the sweep checked the cluster once")
}

// A status write only lands while the row still holds the status_updated value
// the check read: a check that decided from an older report cannot undo a
// newer classification.
func TestNodePoolUpdateStatusLeavesARowAnotherCheckWrote(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "pool-cas")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 1)

	var checkedAt pgtype.Timestamptz
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT status_updated FROM tenant.node_pools WHERE id = $1`, poolID).Scan(&checkedAt))

	rows, err := queries.NodePoolUpdateStatus(t.Context(), dbgen.NodePoolUpdateStatusParams{
		NodePoolID: poolID,
		Status:     pgtype.Text{String: "waiting", Valid: true},
		Message:    pgtype.Text{String: "first", Valid: true},
		CheckedAt:  checkedAt,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	// The same read, replayed after the first write: the row moved on.
	rows, err = queries.NodePoolUpdateStatus(t.Context(), dbgen.NodePoolUpdateStatusParams{
		NodePoolID: poolID,
		Status:     pgtype.Text{String: "error", Valid: true},
		Message:    pgtype.Text{String: "second", Valid: true},
		CheckedAt:  checkedAt,
	})
	require.NoError(t, err)
	require.Zero(t, rows)

	status, message := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, "waiting", status)
	assert.Equal(t, "first", message)
}
