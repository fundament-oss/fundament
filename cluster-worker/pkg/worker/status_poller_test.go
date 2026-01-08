package worker

import (
	"context"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/gardener"
)

func TestStatusPoller_Creation(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	// StatusPoller can be created without a DB connection for basic tests
	// Real functionality requires DB
	cfg := StatusPollerConfig{
		PollInterval: 30 * time.Second,
		BatchSize:    50,
	}

	sp := NewStatusPoller(nil, mock, logger, cfg)
	if sp == nil {
		t.Fatal("status poller should not be nil")
	}
	if sp.cfg.PollInterval != 30*time.Second {
		t.Errorf("expected poll interval 30s, got %v", sp.cfg.PollInterval)
	}
	if sp.cfg.BatchSize != 50 {
		t.Errorf("expected batch size 50, got %d", sp.cfg.BatchSize)
	}
}

func TestStatusPoller_MockGardenerInteraction(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:         uuid.New(),
		Name:       "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Before shoot exists
	status, msg, err := mock.GetShootStatus(ctx, cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected 'pending' status for non-existent shoot, got %q", status)
	}
	if msg != "Shoot not found in Gardener" {
		t.Errorf("unexpected message: %q", msg)
	}

	// Create shoot
	if err := mock.ApplyShoot(ctx, cluster); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// After shoot exists - default is "ready"
	status, _, err = mock.GetShootStatus(ctx, cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "ready" {
		t.Errorf("expected 'ready' status for existing shoot, got %q", status)
	}

	// Delete shoot
	if err := mock.DeleteShoot(ctx, cluster); err != nil {
		t.Fatalf("DeleteShoot failed: %v", err)
	}

	// After shoot deleted
	status, _, err = mock.GetShootStatus(ctx, cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected 'pending' status for deleted shoot, got %q", status)
	}
}

func TestStatusPoller_ProgressingStatus(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:         uuid.New(),
		Name:       "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Create shoot
	if err := mock.ApplyShoot(ctx, cluster); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// Override status to simulate progressing state
	mock.SetStatusOverride(cluster.ID, "progressing", "Creating control plane")

	status, msg, err := mock.GetShootStatus(ctx, cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "progressing" {
		t.Errorf("expected 'progressing' status, got %q", status)
	}
	if msg != "Creating control plane" {
		t.Errorf("expected 'Creating control plane' message, got %q", msg)
	}
}

func TestStatusPoller_ErrorStatus(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:         uuid.New(),
		Name:       "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Create shoot
	if err := mock.ApplyShoot(ctx, cluster); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// Override status to simulate error
	mock.SetStatusOverride(cluster.ID, "error", "Failed to create infrastructure: quota exceeded")

	status, msg, err := mock.GetShootStatus(ctx, cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "error" {
		t.Errorf("expected 'error' status, got %q", status)
	}
	if msg != "Failed to create infrastructure: quota exceeded" {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestStatusPoller_DeletingStatus(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	now := time.Now()
	cluster := gardener.ClusterToSync{
		ID:         uuid.New(),
		Name:       "test-cluster",
		OrganizationName: "test-tenant",
		Deleted:    &now, // Mark as deleted in DB
	}

	// Create shoot (simulating a shoot that's being deleted)
	if err := mock.ApplyShoot(ctx, cluster); err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// Override status to simulate deleting state
	mock.SetStatusOverride(cluster.ID, "deleting", "Deleting control plane")

	status, msg, err := mock.GetShootStatus(ctx, cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "deleting" {
		t.Errorf("expected 'deleting' status, got %q", status)
	}
	if msg != "Deleting control plane" {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestStatusPoller_Integration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration tests")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	logger := testLogger()
	mock := gardener.NewMock(logger)

	sp := NewStatusPoller(pool, mock, logger, StatusPollerConfig{
		PollInterval: 30 * time.Second,
		BatchSize:    50,
	})

	if sp == nil {
		t.Fatal("status poller should not be nil")
	}
}

func TestStatusPoller_RunLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := testLogger()
		mock := gardener.NewMock(logger)

		pollInterval := 100 * time.Millisecond
		pollCount := 0

		// Simulate the Run loop's ticker behavior
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

		// Advance time to trigger multiple polls
		time.Sleep(350 * time.Millisecond)
		synctest.Wait()

		cancel()
		<-done

		// Should have polled at least 3 times (100ms, 200ms, 300ms)
		if pollCount < 3 {
			t.Errorf("expected at least 3 polls, got %d", pollCount)
		}

		// Verify mock wasn't used (we're testing ticker behavior, not actual polling)
		if mock.ListCallCount != 0 {
			t.Error("mock ListShoots should not have been called in this test")
		}
	})
}

func TestStatusPoller_MultipleStatusOverrides(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()

	// Create multiple clusters with different status overrides
	clusters := []struct {
		cluster gardener.ClusterToSync
		status  string
		message string
	}{
		{
			cluster: gardener.ClusterToSync{ID: uuid.New(), Name: "cluster-1", OrganizationName: "tenant"},
			status:  "ready",
			message: "Cluster is ready",
		},
		{
			cluster: gardener.ClusterToSync{ID: uuid.New(), Name: "cluster-2", OrganizationName: "tenant"},
			status:  "progressing",
			message: "Creating workers",
		},
		{
			cluster: gardener.ClusterToSync{ID: uuid.New(), Name: "cluster-3", OrganizationName: "tenant"},
			status:  "error",
			message: "Infrastructure provisioning failed",
		},
	}

	for _, tc := range clusters {
		if err := mock.ApplyShoot(ctx, tc.cluster); err != nil {
			t.Fatalf("ApplyShoot failed: %v", err)
		}
		mock.SetStatusOverride(tc.cluster.ID, tc.status, tc.message)
	}

	// Verify each cluster returns its correct status
	for _, tc := range clusters {
		status, msg, err := mock.GetShootStatus(ctx, tc.cluster)
		if err != nil {
			t.Fatalf("GetShootStatus failed for %s: %v", tc.cluster.Name, err)
		}
		if status != tc.status {
			t.Errorf("cluster %s: expected status %q, got %q", tc.cluster.Name, tc.status, status)
		}
		if msg != tc.message {
			t.Errorf("cluster %s: expected message %q, got %q", tc.cluster.Name, tc.message, msg)
		}
	}
}
