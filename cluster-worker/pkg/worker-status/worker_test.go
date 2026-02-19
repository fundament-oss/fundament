package worker_status

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/common"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

func TestStatusWorker_Creation(t *testing.T) {
	logger := common.TestLogger()

	registry := handler.NewRegistry()
	cfg := Config{
		PollInterval: 30 * time.Second,
	}

	sp := New(registry, logger, cfg)
	if sp == nil {
		t.Fatal("status poller should not be nil")
		return
	}
	if sp.cfg.PollInterval != 30*time.Second {
		t.Errorf("expected poll interval 30s, got %v", sp.cfg.PollInterval)
	}
}

func TestStatusWorker_MockGardenerInteraction(t *testing.T) {
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
		t.Errorf("expected 'ready' status for existing shoot, got %q", shootStatus.Status)
	}

	if err := mock.DeleteShootByClusterID(ctx, cluster.ID); err != nil {
		t.Fatalf("DeleteShootByClusterID failed: %v", err)
	}

	shootStatus, err = mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if shootStatus.Status != gardener.StatusPending || shootStatus.Message != gardener.MsgShootNotFound {
		t.Errorf("expected 'pending' status with 'Shoot not found' for deleted shoot, got %q / %q", shootStatus.Status, shootStatus.Message)
	}
}

func TestStatusWorker_ProgressingStatus(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	mock.SetStatusOverride(cluster.ID, gardener.StatusProgressing, "Creating control plane")

	shootStatus, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if shootStatus.Status != gardener.StatusProgressing {
		t.Errorf("expected 'progressing' status, got %q", shootStatus.Status)
	}
	if shootStatus.Message != "Creating control plane" {
		t.Errorf("expected 'Creating control plane' message, got %q", shootStatus.Message)
	}
}

func TestStatusWorker_ErrorStatus(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	mock.SetStatusOverride(cluster.ID, gardener.StatusError, "Failed to create infrastructure: quota exceeded")

	shootStatus, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if shootStatus.Status != gardener.StatusError {
		t.Errorf("expected 'error' status, got %q", shootStatus.Status)
	}
	if shootStatus.Message != "Failed to create infrastructure: quota exceeded" {
		t.Errorf("unexpected message: %q", shootStatus.Message)
	}
}

func TestStatusWorker_DeletingStatus(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	now := time.Now()
	cluster := common.TestCluster("test-cluster", "test-tenant")
	cluster.Deleted = &now

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	mock.SetStatusOverride(cluster.ID, gardener.StatusDeleting, "Deleting control plane")

	shootStatus, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if shootStatus.Status != gardener.StatusDeleting {
		t.Errorf("expected 'deleting' status, got %q", shootStatus.Status)
	}
	if shootStatus.Message != "Deleting control plane" {
		t.Errorf("unexpected message: %q", shootStatus.Message)
	}
}

func TestStatusWorker_RunLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pollInterval := 100 * time.Millisecond
		pollCount := 0

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-ctx.Done():
					close(done)
					return
				case <-ticker.C:
					pollCount++
				}
			}
		}()

		time.Sleep(350 * time.Millisecond)
		synctest.Wait()

		cancel()
		<-done

		if pollCount < 3 {
			t.Errorf("expected at least 3 polls, got %d", pollCount)
		}
	})
}

func TestStatusWorker_MultipleStatusOverrides(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()

	clusters := []struct {
		cluster gardener.ClusterToSync
		status  gardener.ShootStatusType
		message string
	}{
		{
			cluster: common.TestCluster("cluster-1", "tenant"),
			status:  gardener.StatusReady,
			message: "Cluster is ready",
		},
		{
			cluster: common.TestCluster("cluster-2", "tenant"),
			status:  gardener.StatusProgressing,
			message: "Creating workers",
		},
		{
			cluster: common.TestCluster("cluster-3", "tenant"),
			status:  gardener.StatusError,
			message: "Infrastructure provisioning failed",
		},
	}

	for i := range clusters {
		err := mock.ApplyShoot(ctx, &clusters[i].cluster)
		if err != nil {
			t.Fatalf("ApplyShoot failed: %v", err)
		}
		mock.SetStatusOverride(clusters[i].cluster.ID, clusters[i].status, clusters[i].message)
	}

	for _, tc := range clusters {
		shootStatus, err := mock.GetShootStatus(ctx, &tc.cluster)
		if err != nil {
			t.Fatalf("GetShootStatus failed for %s: %v", tc.cluster.Name, err)
		}
		if shootStatus.Status != tc.status {
			t.Errorf("cluster %s: expected status %q, got %q", tc.cluster.Name, tc.status, shootStatus.Status)
		}
		if shootStatus.Message != tc.message {
			t.Errorf("cluster %s: expected message %q, got %q", tc.cluster.Name, tc.message, shootStatus.Message)
		}
	}
}
