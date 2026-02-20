package cluster_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/common"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
)

func TestReconcileOrphans_DeletesOrphans(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// Create a shoot in Gardener that has no corresponding cluster in the DB.
	orphanCluster := common.TestCluster("orphan-cluster", "ghost-org")
	if err := mock.ApplyShoot(ctx, &orphanCluster); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	if err := h.ReconcileOrphans(ctx); err != nil {
		t.Fatalf("ReconcileOrphans failed: %v", err)
	}

	if mock.HasShootForCluster(orphanCluster.ID) {
		t.Error("expected orphaned shoot to be deleted")
	}
	if len(mock.DeleteByClusterID) != 1 {
		t.Errorf("expected 1 delete call, got %d", len(mock.DeleteByClusterID))
	}
}

func TestReconcileOrphans_KeepsKnownClusters(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// Insert a real cluster in the DB.
	orgID := uuid.New()
	clusterID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO tenant.organizations (id, name) VALUES ($1, $2)", orgID, "test-org")
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version) VALUES ($1, $2, $3, $4, $5)",
		clusterID, orgID, "real-cluster", "local", "1.31.1")
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Create a shoot in Gardener for this cluster.
	shoot := gardener.ClusterToSync{
		ID:                clusterID,
		OrganizationID:    orgID,
		OrganizationName:  "test-org",
		Name:              "real-cluster",
		ShootName:         gardener.GenerateShootName("real-cluster"),
		Namespace:         gardener.NamespaceFromProjectName(gardener.ProjectName("test-org")),
		Region:            "local",
		KubernetesVersion: "1.31.1",
	}
	if err := mock.ApplyShoot(ctx, &shoot); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	if err := h.ReconcileOrphans(ctx); err != nil {
		t.Fatalf("ReconcileOrphans failed: %v", err)
	}

	if !mock.HasShootForCluster(clusterID) {
		t.Error("expected shoot for known cluster to be kept")
	}
	if len(mock.DeleteByClusterID) != 0 {
		t.Errorf("expected 0 delete calls, got %d", len(mock.DeleteByClusterID))
	}
}

func TestReconcileOrphans_KeepsSoftDeletedClusters(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// Insert a soft-deleted cluster.
	orgID := uuid.New()
	clusterID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO tenant.organizations (id, name) VALUES ($1, $2)", orgID, "test-org")
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version, deleted) VALUES ($1, $2, $3, $4, $5, now())",
		clusterID, orgID, "deleted-cluster", "local", "1.31.1")
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Create a shoot for the soft-deleted cluster.
	shoot := gardener.ClusterToSync{
		ID:                clusterID,
		OrganizationID:    orgID,
		OrganizationName:  "test-org",
		Name:              "deleted-cluster",
		ShootName:         gardener.GenerateShootName("deleted-cluster"),
		Namespace:         gardener.NamespaceFromProjectName(gardener.ProjectName("test-org")),
		Region:            "local",
		KubernetesVersion: "1.31.1",
	}
	if err := mock.ApplyShoot(ctx, &shoot); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	if err := h.ReconcileOrphans(ctx); err != nil {
		t.Fatalf("ReconcileOrphans failed: %v", err)
	}

	// Soft-deleted cluster should NOT be treated as an orphan.
	if !mock.HasShootForCluster(clusterID) {
		t.Error("expected shoot for soft-deleted cluster to be kept (not orphaned)")
	}
	if len(mock.DeleteByClusterID) != 0 {
		t.Errorf("expected 0 delete calls, got %d", len(mock.DeleteByClusterID))
	}
}

func TestReconcileOrphans_EmptyShoots(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// No shoots in Gardener — should be a no-op.
	if err := h.ReconcileOrphans(ctx); err != nil {
		t.Fatalf("ReconcileOrphans failed: %v", err)
	}

	if len(mock.DeleteByClusterID) != 0 {
		t.Errorf("expected 0 delete calls, got %d", len(mock.DeleteByClusterID))
	}
}
