package cluster_test

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
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

func TestReconcileKeepsShootWithoutCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// A shoot for a cluster ID that has no row in the DB, as after a database
	// reset or for a shoot of another installation.
	unknownClusterID := uuid.New()
	applyTestShoot(t, mock, unknownClusterID, "unknown")

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	assert.Empty(t, mock.DeleteByClusterID)
	assert.True(t, mock.HasShootForCluster(unknownClusterID))
}

func TestReconcileDeletesShootOfDeletedCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// The cluster was deleted by a user, the delete sync gave up, and its
	// shoot still exists.
	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "reconcile-deleted")
	markOutboxFailed(t, db, clusterID)
	applyTestShoot(t, mock, clusterID, "reconcile-deleted")

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{clusterID}, mock.DeleteByClusterID)
}

func TestReconcileLeavesShootWhileDeleteSyncRuns(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// The cluster was just deleted: its outbox row is still pending, so the
	// delete sync has yet to delete the shoot.
	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "reconcile-syncing")
	applyTestShoot(t, mock, clusterID, "reconcile-syncing")

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	assert.Empty(t, mock.DeleteByClusterID)
	assert.True(t, mock.HasShootForCluster(clusterID))
}

func TestReconcileRefusesMassDeletion(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// One more deleted cluster with a shoot than a pass may delete.
	for i := range 4 {
		name := fmt.Sprintf("reconcile-mass-%d", i)
		clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, name)
		markOutboxFailed(t, db, clusterID)
		applyTestShoot(t, mock, clusterID, name)
	}

	// Refusing is logged, not returned: a failed pass counts towards stopping
	// the worker.
	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	assert.Empty(t, mock.DeleteByClusterID)
}

func TestReconcileRefusedPassStillConfirmsDeletingShoots(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	mock.DeleteDelay = time.Hour // keep deleting shoots listed
	h := newTestHandler(t, db, mock)

	// A shoot whose deletion is already under way.
	deletingID := insertDeletedCluster(t, db, acmeCorpOrgID, "reconcile-refused-deleting")
	applyTestShoot(t, mock, deletingID, "reconcile-refused-deleting")
	require.NoError(t, mock.DeleteShootByClusterID(t.Context(), deletingID))
	markOutboxCompleted(t, db, deletingID)

	// One more new deletion than a pass may start.
	for i := range 4 {
		name := fmt.Sprintf("reconcile-refused-%d", i)
		clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, name)
		markOutboxFailed(t, db, clusterID)
		applyTestShoot(t, mock, clusterID, name)
	}

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	// Only the deleting shoot is confirmed again, on top of the setup's delete.
	assert.Equal(t, []uuid.UUID{deletingID, deletingID}, mock.DeleteByClusterID)
}

func TestReconcileMassDeletionIgnoresShootsAlreadyDeleting(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	mock.DeleteDelay = time.Hour // keep deleting shoots listed
	h := newTestHandler(t, db, mock)

	// Four deleted clusters whose shoots are already being deleted, as after
	// a user deleted them through the sync path.
	var clusterIDs []uuid.UUID
	for i := range 4 {
		name := fmt.Sprintf("reconcile-deleting-%d", i)
		clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, name)
		applyTestShoot(t, mock, clusterID, name)
		require.NoError(t, mock.DeleteShootByClusterID(t.Context(), clusterID))
		markOutboxCompleted(t, db, clusterID)
		clusterIDs = append(clusterIDs, clusterID)
	}

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	// The deletion is repeated for each, on top of the four from the setup.
	assert.Len(t, mock.DeleteByClusterID, 8)
	assert.ElementsMatch(t, clusterIDs, mock.DeleteByClusterID[4:])
}

func applyTestShoot(t *testing.T, mock *gardener.MockClient, clusterID uuid.UUID, name string) {
	t.Helper()

	_, err := mock.ApplyShoot(t.Context(), &gardener.ClusterToSync{
		ID:                clusterID,
		OrganizationID:    acmeCorpOrgID,
		OrganizationName:  "acme-corp",
		Name:              name,
		ShootName:         kubename.GenerateShootName(name, clusterID),
		Namespace:         "garden-test",
		Region:            "eu-west-1",
		KubernetesVersion: "1.31.1",
	})
	require.NoError(t, err)
}

// Drift repair's "latest sync completed" is the organization API's sync state:
// OutboxInsertReconcile repairs a cluster exactly when the cluster_sync_state
// view's row completed and nothing is queued. Every event the database allows
// is tried, so a new event counts as a sync on both sides or on neither.
func TestClusterReconcileAgreesWithClusterSyncState(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	var constraint string
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT pg_get_constraintdef(pg_constraint.oid) FROM pg_constraint
		 WHERE pg_constraint.conname = 'cluster_outbox_ck_event'`).Scan(&constraint))
	var events []string
	for _, match := range regexp.MustCompile(`'([a-z_]+)'::text`).FindAllStringSubmatch(constraint, -1) {
		events = append(events, match[1])
	}
	require.NotEmpty(t, events)

	n := 0
	check := func(name string, setup func(clusterID uuid.UUID)) {
		n++
		clusterID := insertCluster(t, db, acmeCorpOrgID, fmt.Sprintf("agree-%d", n))
		setup(clusterID)

		var viewStatus *string
		err := db.adminPool.QueryRow(t.Context(),
			`SELECT tenant.cluster_sync_state.outbox_status FROM tenant.cluster_sync_state
			 WHERE tenant.cluster_sync_state.cluster_id = $1`, clusterID).Scan(&viewStatus)
		require.NoError(t, err, name)
		var queued bool
		require.NoError(t, db.adminPool.QueryRow(t.Context(),
			`SELECT EXISTS (SELECT 1 FROM tenant.cluster_outbox
			 WHERE tenant.cluster_outbox.cluster_id = $1 AND tenant.cluster_outbox.status IN ('pending', 'retrying'))`,
			clusterID).Scan(&queued), name)
		want := viewStatus != nil && *viewStatus == "completed" && !queued

		inserted, err := queries.OutboxInsertReconcile(t.Context(), dbgen.OutboxInsertReconcileParams{
			ClusterID: pgtype.UUID{Bytes: clusterID, Valid: true},
		})
		require.NoError(t, err, name)
		require.Equalf(t, want, inserted == 1, "%s: view status %v, queued %v", name, viewStatus, queued)
	}

	check("never synced", func(uuid.UUID) {})
	for _, event := range events {
		for _, status := range []string{"pending", "failed", "completed"} {
			check(event+" "+status+" after a completed create", func(clusterID uuid.UUID) {
				markOutboxCompleted(t, db, clusterID)
				_, err := db.adminPool.Exec(t.Context(),
					`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status) VALUES ($1, $2, 'trigger', $3)`,
					clusterID, event, status)
				require.NoError(t, err)
			})
		}
	}
	check("a node pool's failure is not the cluster's", func(clusterID uuid.UUID) {
		markOutboxCompleted(t, db, clusterID)
		poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 5)
		_, err := db.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = 'failed', failed = now() WHERE node_pool_id = $1`, poolID)
		require.NoError(t, err)
	})
}
