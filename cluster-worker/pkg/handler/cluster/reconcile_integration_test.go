package cluster_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
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

func TestReconcileOrphanCleanup(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Create a shoot in Gardener for a cluster ID that doesn't exist in the DB.
	orphanClusterID := uuid.New()
	orphanShootName := gardener.GenerateShootName("orphan", orphanClusterID)
	err := mock.ApplyShoot(t.Context(), &gardener.ClusterToSync{
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
