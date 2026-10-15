package cluster_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
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
