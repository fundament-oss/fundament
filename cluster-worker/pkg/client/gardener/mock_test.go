package gardener_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/common"
)

func TestMockClient_ApplyShoot(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	if !mock.HasShootForCluster(cluster.ID) {
		t.Errorf("expected shoot for cluster %s to exist", cluster.ID)
	}

	if len(mock.ApplyCalls) != 1 {
		t.Errorf("expected 1 apply call, got %d", len(mock.ApplyCalls))
	}
	if mock.ApplyCalls[0].ID != cluster.ID {
		t.Error("apply call did not match cluster")
	}
}

func TestMockClient_DeleteShootByClusterID(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	err = mock.DeleteShootByClusterID(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("DeleteShootByClusterID failed: %v", err)
	}

	if mock.HasShootForCluster(cluster.ID) {
		t.Error("expected shoot to be marked deleted")
	}

	if len(mock.DeleteByClusterID) != 1 {
		t.Errorf("expected 1 delete call, got %d", len(mock.DeleteByClusterID))
	}
}

func TestMockClient_ListShoots(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		cluster := common.TestCluster("cluster-"+string(rune('a'+i)), "tenant")
		err := mock.ApplyShoot(ctx, &cluster)
		if err != nil {
			t.Fatalf("ApplyShoot failed: %v", err)
		}
	}

	shoots, err := mock.ListShoots(ctx)
	if err != nil {
		t.Fatalf("ListShoots failed: %v", err)
	}

	if len(shoots) != 3 {
		t.Errorf("expected 3 shoots, got %d", len(shoots))
	}
}

func TestMockClient_GetShootStatus(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	shootStatus, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if shootStatus.Status != gardener.StatusReady {
		t.Errorf("expected status 'ready', got %q", shootStatus.Status)
	}
	if shootStatus.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestMockClient_StatusOverride(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	mock.SetStatusOverride(cluster.ID, gardener.StatusProgressing, "Creating infrastructure")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	shootStatus, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if shootStatus.Status != gardener.StatusProgressing {
		t.Errorf("expected status 'progressing', got %q", shootStatus.Status)
	}
	if shootStatus.Message != "Creating infrastructure" {
		t.Errorf("expected message 'Creating infrastructure', got %q", shootStatus.Message)
	}
}

func TestMockClient_ApplyError(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	mock.SetApplyError(gardener.ErrMockApplyFailed)

	err := mock.ApplyShoot(ctx, &cluster)
	if !errors.Is(err, gardener.ErrMockApplyFailed) {
		t.Errorf("expected ErrMockApplyFailed, got %v", err)
	}

	if mock.ShootCount() != 0 {
		t.Error("shoot should not exist after error")
	}
}

func TestMockClient_Reset(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}
	mock.SetApplyError(gardener.ErrMockApplyFailed)

	mock.Reset()

	if mock.ShootCount() != 0 {
		t.Error("expected 0 shoots after reset")
	}
	if len(mock.ApplyCalls) != 0 {
		t.Error("expected 0 apply calls after reset")
	}

	err = mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}
