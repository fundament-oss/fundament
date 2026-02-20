package cluster_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/common"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
)

func TestCheckStatus_ActiveClusterGetsStatusUpdated(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// Insert an org and a synced cluster (active, shoot_status = NULL).
	orgID := uuid.New()
	clusterID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO tenant.organizations (id, name) VALUES ($1, $2)", orgID, "status-org")
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version, synced) VALUES ($1, $2, $3, $4, $5, now())",
		clusterID, orgID, "status-cluster", "local", "1.31.1")
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Create a shoot in mock so GetShootStatus returns ready.
	shoot := gardener.ClusterToSync{
		ID:                clusterID,
		OrganizationID:    orgID,
		OrganizationName:  "status-org",
		Name:              "status-cluster",
		ShootName:         gardener.GenerateShootName("status-cluster"),
		Namespace:         gardener.NamespaceFromProjectName(gardener.ProjectName("status-org")),
		Region:            "local",
		KubernetesVersion: "1.31.1",
	}
	if err := mock.ApplyShoot(ctx, &shoot); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	if err := h.CheckStatus(ctx); err != nil {
		t.Fatalf("CheckStatus failed: %v", err)
	}

	// Verify shoot_status was updated in the DB.
	var shootStatus *string
	err = pool.QueryRow(ctx, "SELECT shoot_status FROM tenant.clusters WHERE id = $1", clusterID).Scan(&shootStatus)
	if err != nil {
		t.Fatalf("failed to query shoot_status: %v", err)
	}
	if shootStatus == nil || *shootStatus != string(gardener.StatusReady) {
		t.Errorf("expected shoot_status 'ready', got %v", shootStatus)
	}

	// Verify a status_ready event was created.
	var eventCount int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM tenant.cluster_events WHERE cluster_id = $1 AND event_type = 'status_ready'",
		clusterID).Scan(&eventCount)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected 1 status_ready event, got %d", eventCount)
	}
}

func TestCheckStatus_DeletedClusterConfirmedGone(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// Insert org + soft-deleted cluster that was synced.
	orgID := uuid.New()
	clusterID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO tenant.organizations (id, name) VALUES ($1, $2)", orgID, "del-org")
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version, synced, deleted) VALUES ($1, $2, $3, $4, $5, now(), now())",
		clusterID, orgID, "del-cluster", "local", "1.31.1")
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Don't create a shoot — mock.GetShootStatus will return StatusPending + MsgShootNotFound.

	if err := h.CheckStatus(ctx); err != nil {
		t.Fatalf("CheckStatus failed: %v", err)
	}

	// Verify shoot_status was set to 'deleted'.
	var shootStatus *string
	err = pool.QueryRow(ctx, "SELECT shoot_status FROM tenant.clusters WHERE id = $1", clusterID).Scan(&shootStatus)
	if err != nil {
		t.Fatalf("failed to query shoot_status: %v", err)
	}
	if shootStatus == nil || *shootStatus != string(gardener.StatusDeleted) {
		t.Errorf("expected shoot_status 'deleted', got %v", shootStatus)
	}

	// Verify a status_deleted event was created.
	var eventCount int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM tenant.cluster_events WHERE cluster_id = $1 AND event_type = 'status_deleted'",
		clusterID).Scan(&eventCount)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected 1 status_deleted event, got %d", eventCount)
	}
}

func TestCheckStatus_DeletedClusterStillDeleting(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// Insert org + soft-deleted, synced cluster.
	orgID := uuid.New()
	clusterID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO tenant.organizations (id, name) VALUES ($1, $2)", orgID, "deleting-org")
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version, synced, deleted) VALUES ($1, $2, $3, $4, $5, now(), now())",
		clusterID, orgID, "deleting-cluster", "local", "1.31.1")
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Create a shoot and override status to deleting — shoot still exists.
	shoot := gardener.ClusterToSync{
		ID:                clusterID,
		OrganizationID:    orgID,
		OrganizationName:  "deleting-org",
		Name:              "deleting-cluster",
		ShootName:         gardener.GenerateShootName("deleting-cluster"),
		Namespace:         gardener.NamespaceFromProjectName(gardener.ProjectName("deleting-org")),
		Region:            "local",
		KubernetesVersion: "1.31.1",
	}
	if err := mock.ApplyShoot(ctx, &shoot); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}
	mock.SetStatusOverride(clusterID, gardener.StatusDeleting, "Shoot is being deleted")

	if err := h.CheckStatus(ctx); err != nil {
		t.Fatalf("CheckStatus failed: %v", err)
	}

	// Verify shoot_status was set to 'deleting' (not yet confirmed deleted).
	var shootStatus *string
	err = pool.QueryRow(ctx, "SELECT shoot_status FROM tenant.clusters WHERE id = $1", clusterID).Scan(&shootStatus)
	if err != nil {
		t.Fatalf("failed to query shoot_status: %v", err)
	}
	if shootStatus == nil || *shootStatus != string(gardener.StatusDeleting) {
		t.Errorf("expected shoot_status 'deleting', got %v", shootStatus)
	}
}

func TestCheckStatus_AllFailsReturnsError(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// Insert org + synced cluster.
	orgID := uuid.New()
	clusterID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO tenant.organizations (id, name) VALUES ($1, $2)", orgID, "fail-org")
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version, synced) VALUES ($1, $2, $3, $4, $5, now())",
		clusterID, orgID, "fail-cluster", "local", "1.31.1")
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Configure mock to fail on GetShootStatus.
	mock.GetStatusError = fmt.Errorf("gardener unavailable")

	err = h.CheckStatus(ctx)
	if err == nil {
		t.Error("expected CheckStatus to return error when all checks fail")
	}
}

func TestCheckStatus_NoClustersIsNoOp(t *testing.T) {
	t.Parallel()
	pool := createTestDB(t)
	ctx := context.Background()
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	queries := db.New(pool)
	h := cluster.New(queries, mock, logger)

	// No clusters in DB — should be a no-op.
	if err := h.CheckStatus(ctx); err != nil {
		t.Fatalf("CheckStatus failed: %v", err)
	}

	if len(mock.StatusCalls) != 0 {
		t.Errorf("expected 0 status calls, got %d", len(mock.StatusCalls))
	}
}
