package cluster_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	dbgen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/kubename"
)

func TestReconcileDriftDetected(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert cluster and mark its outbox as completed (simulates a previously synced cluster).
	clusterID := insertCluster(t, db, acmeCorpOrgID, "reconcile-drift")
	markOutboxCompleted(t, db, clusterID)

	// Mock returns no shoots — drift: DB says synced but Gardener has nothing.
	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	// A reconcile outbox row should have been inserted.
	assertOutboxReconcileExists(t, db, clusterID)
}

func TestReconcileNoDriftForUnsyncedCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert cluster — outbox starts as pending (trigger default), never completed.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "reconcile-unsynced")

	// Mock returns no shoots.
	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	// No reconcile row — cluster was never synced, so no drift.
	assertNoOutboxReconcile(t, db, clusterID)
}

func TestReconcileNoDriftAfterExhaustedReconcile(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// The cluster reached Gardener once; its reconcile then failed for good.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "reconcile-exhausted")
	markOutboxCompleted(t, db, clusterID)
	insertFailedReconcile(t, db, clusterID)

	// Mock returns no shoots — the Shoot is still missing.
	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	// No new reconcile row — the exhausted one is the cluster's latest sync.
	require.Equal(t, 1, countOutboxReconcile(t, db, clusterID))
}

func TestReconcileOrphanCleanup(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Create a shoot in Gardener for a cluster ID that doesn't exist in the DB.
	orphanClusterID := uuid.New()
	orphanShootName := kubename.GenerateShootName("orphan", orphanClusterID)
	_, err := mock.ApplyShoot(t.Context(), &gardener.ClusterToSync{
		ID:                orphanClusterID,
		OrganizationID:    acmeCorpOrgID,
		OrganizationName:  "acme-corp",
		Name:              "orphan",
		ShootName:         orphanShootName,
		Namespace:         "garden-test",
		Region:            "eu-west-1",
		KubernetesVersion: "1.31.1",
	})
	require.NoError(t, err)

	err = h.Reconcile(t.Context())
	require.NoError(t, err)

	// Orphan shoot should have been deleted.
	require.Len(t, mock.DeleteByClusterID, 1)
	require.Equal(t, orphanClusterID, mock.DeleteByClusterID[0])
}

// Drift repair's "latest sync completed" is the organization API's sync state:
// ClusterListActive picks the same row as the cluster_sync_state view, so the
// two cannot disagree on a cluster.
func TestClusterListActiveAgreesWithClusterSyncState(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	// addRow adds a cluster outbox row queued after the ones before it.
	addRow := func(clusterID uuid.UUID, event, status string, minutesAgo int) {
		_, err := db.adminPool.Exec(t.Context(),
			`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status, created)
			 VALUES ($1, $2, 'trigger', $3, now() - make_interval(mins => $4))`,
			clusterID, event, status, minutesAgo)
		require.NoError(t, err)
	}
	failNodePoolRows := func(nodePoolID uuid.UUID) {
		_, err := db.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = 'failed', failed = now() WHERE node_pool_id = $1`, nodePoolID)
		require.NoError(t, err)
	}

	cases := map[string]func(clusterID uuid.UUID){
		"created and synced": func(uuid.UUID) {},
		"a newer ready fan-out row is not a sync": func(id uuid.UUID) {
			addRow(id, "ready", "pending", 0)
		},
		"a newer reconcile failed for good": func(id uuid.UUID) {
			addRow(id, "reconcile", "failed", 0)
		},
		"a newer update still pending": func(id uuid.UUID) {
			addRow(id, "updated", "pending", 0)
		},
		"a node pool's failure is not the cluster's": func(id uuid.UUID) {
			failNodePoolRows(insertNodePoolReturningID(t, db, id, "workers", "n1-standard-4", 1, 5))
		},
	}
	want := map[uuid.UUID]string{}
	i := 0
	for name, setup := range cases {
		i++
		clusterID := insertCluster(t, db, acmeCorpOrgID, fmt.Sprintf("agree-%d", i))
		_, err := db.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = 'completed', created = now() - interval '1 hour' WHERE cluster_id = $1`, clusterID)
		require.NoError(t, err)
		setup(clusterID)
		want[clusterID] = name
	}
	never := insertCluster(t, db, acmeCorpOrgID, "agree-never-synced")
	want[never] = "never synced"

	rows, err := queries.ClusterListActive(t.Context())
	require.NoError(t, err)
	seen := 0
	for _, row := range rows {
		name, ok := want[row.ID]
		if !ok {
			continue
		}
		seen++
		var viewStatus *string
		err := db.adminPool.QueryRow(t.Context(),
			`SELECT tenant.cluster_sync_state.outbox_status FROM tenant.cluster_sync_state
			 WHERE tenant.cluster_sync_state.cluster_id = $1`, row.ID).Scan(&viewStatus)
		require.NoError(t, err, name)
		require.NotNil(t, viewStatus, name)
		require.Equalf(t, *viewStatus == "completed", row.HasCompletedOutbox,
			"%s: view says %q, ClusterListActive says completed=%v", name, *viewStatus, row.HasCompletedOutbox)
	}
	require.Equal(t, len(want), seen)
}
