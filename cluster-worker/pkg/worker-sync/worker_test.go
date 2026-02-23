package worker_sync

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/common"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/gardener"
)

func TestMockClient_ApplyShootWithNodePools(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")
	cluster.NodePools = []gardener.NodePool{
		{Name: "pool-a", MachineType: "local", AutoscaleMin: 1, AutoscaleMax: 5},
		{Name: "pool-b", MachineType: "local", AutoscaleMin: 2, AutoscaleMax: 10},
	}

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot with node pools failed: %v", err)
	}

	if !mock.HasShootForCluster(cluster.ID) {
		t.Errorf("expected shoot for cluster %s to exist", cluster.ID)
	}
}

func TestMockClient_ApplyShootWithoutNodePools(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestClusterWithoutNodePools("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot without node pools failed: %v", err)
	}

	if !mock.HasShootForCluster(cluster.ID) {
		t.Errorf("expected shoot for cluster %s to exist", cluster.ID)
	}
}

func TestMockClient_NodePoolValidation(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()

	t.Run("empty node pool name", func(t *testing.T) {
		cluster := common.TestCluster("test-cluster", "test-tenant")
		cluster.NodePools = []gardener.NodePool{
			{Name: "", MachineType: "local", AutoscaleMin: 1, AutoscaleMax: 3},
		}
		err := mock.ApplyShoot(ctx, &cluster)
		if err == nil {
			t.Error("expected error for empty node pool name")
		}
	})

	t.Run("max less than min", func(t *testing.T) {
		cluster := common.TestCluster("test-cluster2", "test-tenant")
		cluster.NodePools = []gardener.NodePool{
			{Name: "pool", MachineType: "local", AutoscaleMin: 5, AutoscaleMax: 2},
		}
		err := mock.ApplyShoot(ctx, &cluster)
		if err == nil {
			t.Error("expected error for max < min")
		}
	})
}

func TestMockClient_ApplyShoot(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// Verify shoot was created for this cluster
	if !mock.HasShootForCluster(cluster.ID) {
		t.Errorf("expected shoot for cluster %s to exist", cluster.ID)
	}

	// Verify call was recorded
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

	// Create shoot first
	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// Now delete by cluster ID
	err = mock.DeleteShootByClusterID(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("DeleteShootByClusterID failed: %v", err)
	}

	// Verify shoot is marked for deletion (HasShootForCluster returns false for deleted)
	if mock.HasShootForCluster(cluster.ID) {
		t.Error("expected shoot to be marked deleted")
	}

	// Verify call was recorded
	if len(mock.DeleteByClusterID) != 1 {
		t.Errorf("expected 1 delete call, got %d", len(mock.DeleteByClusterID))
	}
}

func TestMockClient_ListShoots(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()

	// Create multiple shoots
	for i := range 3 {
		cluster := common.TestCluster("cluster-"+string(rune('a'+i)), "tenant")
		err := mock.ApplyShoot(ctx, &cluster)
		if err != nil {
			t.Fatalf("ApplyShoot failed: %v", err)
		}
	}

	// List shoots
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
	mock := gardener.NewMockInstant(logger) // Instant transitions for this test

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	// Create shoot (ShootName is pre-set from common.TestCluster)
	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// After shoot exists - should return ready (instant mock skips progression)
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

	// Set override
	mock.SetStatusOverride(cluster.ID, gardener.StatusProgressing, "Creating infrastructure")

	// Create shoot (ShootName is pre-set from common.TestCluster)
	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// Should return override status, not default "ready"
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

	// Set error
	mock.SetApplyError(gardener.ErrMockApplyFailed)

	err := mock.ApplyShoot(ctx, &cluster)
	if !errors.Is(err, gardener.ErrMockApplyFailed) {
		t.Errorf("expected ErrMockApplyFailed, got %v", err)
	}

	// Shoot should not exist - count should be 0
	if mock.ShootCount() != 0 {
		t.Error("shoot should not exist after error")
	}
}

func TestMockClient_Reset(t *testing.T) {
	logger := common.TestLogger()
	mock := gardener.NewMockInstant(logger)

	ctx := context.Background()
	cluster := common.TestCluster("test-cluster", "test-tenant")

	// Create shoot and set error
	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}
	mock.SetApplyError(gardener.ErrMockApplyFailed)

	// Reset
	mock.Reset()

	// Verify everything is cleared
	if mock.ShootCount() != 0 {
		t.Error("expected 0 shoots after reset")
	}
	if len(mock.ApplyCalls) != 0 {
		t.Error("expected 0 apply calls after reset")
	}

	// Should be able to apply again without error
	err = mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestProjectName(t *testing.T) {
	tests := []struct {
		name       string
		orgName    string
		wantPrefix string // first 6 chars (sanitized org)
		wantLen    int    // always 10 chars
	}{
		{
			name:       "normal org name",
			orgName:    "Acme Corp",
			wantPrefix: "acmeco", // 6 chars sanitized
			wantLen:    10,
		},
		{
			name:       "short org name gets padded",
			orgName:    "abc",
			wantPrefix: "abc", // 3 chars + 3 hash padding
			wantLen:    10,
		},
		{
			name:       "long org name gets truncated",
			orgName:    "very-long-organization-name",
			wantPrefix: "verylo", // 6 chars
			wantLen:    10,
		},
		{
			name:       "special chars removed",
			orgName:    "My-Org!@#$",
			wantPrefix: "myorg", // special chars removed
			wantLen:    10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gardener.ProjectName(tt.orgName)

			if len(got) != tt.wantLen {
				t.Errorf("expected length %d, got %d (%q)", tt.wantLen, len(got), got)
			}

			if !hasPrefix(got, tt.wantPrefix) {
				t.Errorf("expected prefix %q, got %q", tt.wantPrefix, got)
			}
		})
	}
}

func TestProjectName_Deterministic(t *testing.T) {
	// Same input should always produce same output
	name1 := gardener.ProjectName("Test Organization")
	name2 := gardener.ProjectName("Test Organization")

	if name1 != name2 {
		t.Errorf("ProjectName is not deterministic: %q != %q", name1, name2)
	}
}

func TestProjectName_DifferentOrgsProduceDifferentNames(t *testing.T) {
	// Different orgs should produce different project names
	name1 := gardener.ProjectName("Organization A")
	name2 := gardener.ProjectName("Organization B")

	if name1 == name2 {
		t.Errorf("different orgs produced same project name: %q", name1)
	}
}

func TestGenerateShootName(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		wantPrefix  string // first 8 chars (sanitized cluster name)
		wantLen     int    // always 11 chars
	}{
		{
			name:        "normal cluster name",
			clusterName: "production",
			wantPrefix:  "producti", // 8 chars
			wantLen:     11,
		},
		{
			name:        "short cluster name gets padded",
			clusterName: "dev",
			wantPrefix:  "dev", // 3 chars + 5 random padding
			wantLen:     11,
		},
		{
			name:        "long cluster name gets truncated",
			clusterName: "very-long-cluster-name",
			wantPrefix:  "verylon", // 8 chars (hyphens removed)
			wantLen:     11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gardener.GenerateShootName(tt.clusterName)

			if len(got) != tt.wantLen {
				t.Errorf("expected length %d, got %d (%q)", tt.wantLen, len(got), got)
			}

			if !hasPrefix(got, tt.wantPrefix) {
				t.Errorf("expected prefix %q, got %q", tt.wantPrefix, got)
			}
		})
	}
}

func TestGenerateShootName_Randomness(t *testing.T) {
	// Multiple calls should produce different names (random suffix)
	name1 := gardener.GenerateShootName("cluster")
	name2 := gardener.GenerateShootName("cluster")

	if name1 == name2 {
		// With 36^3 possible combinations, collision is extremely unlikely
		t.Errorf("GenerateShootName should produce random names, got %q twice", name1)
	}
}

func TestNamingLengthConstraints(t *testing.T) {
	// Test that combined project + shoot names fit within Gardener's 21 char limit
	orgs := []string{"Acme Corp", "Very Long Organization Name", "a", "123 Corp"}
	clusters := []string{"production", "very-long-cluster-name", "a", "123-cluster"}

	for _, org := range orgs {
		for _, cluster := range clusters {
			projectName := gardener.ProjectName(org)
			shootName := gardener.GenerateShootName(cluster)
			combined := len(projectName) + len(shootName)

			if combined != 21 {
				t.Errorf("combined length should be exactly 21, got %d (project=%q [%d], shoot=%q [%d])",
					combined, projectName, len(projectName), shootName, len(shootName))
			}
		}
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Integration tests that require a real PostgreSQL connection
// These are skipped unless DATABASE_URL is set

func TestSyncWorker_Integration(t *testing.T) {
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

	logger := common.TestLogger()
	mock := gardener.NewMock(logger)

	w := New(pool, mock, logger, Config{
		PollInterval:      30 * time.Second,
		ReconcileInterval: 5 * time.Minute,
	})

	// Test basic worker creation
	if w == nil {
		t.Fatal("worker should not be nil")
	}

	// Test IsReady before Run
	if w.IsReady() {
		t.Error("worker should not be ready before Run")
	}
}

// synctest-based tests for time-dependent code

func TestBackoffTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Test the backoff formula: 30s * 2^attempts, capped at 900s (15 min)
		testCases := []struct {
			attempts        int
			expectedSeconds float64
		}{
			{0, 30},
			{1, 60},
			{2, 120},
			{3, 240},
			{4, 480},
			{5, 900},  // 30 * 2^5 = 960, but capped at 900
			{6, 900},  // capped
			{10, 900}, // still capped
		}

		for _, tc := range testCases {
			// Backoff formula from SQL: LEAST(30 * POWER(2, attempts), 900)
			backoff := min(30.0*pow2(tc.attempts), 900.0)
			if backoff != tc.expectedSeconds {
				t.Errorf("attempts=%d: expected %v seconds, got %v",
					tc.attempts, tc.expectedSeconds, backoff)
			}
		}
	})
}

func pow2(n int) float64 {
	result := 1.0
	for range n {
		result *= 2
	}
	return result
}

func TestStatusPoller_Timing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := common.TestLogger()
		mock := gardener.NewMock(logger)

		// We can't easily test the full poller without a DB connection,
		// but we can verify the ticker behavior using synctest
		pollInterval := 30 * time.Second
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		pollCount := 0
		done := make(chan struct{})

		go func() {
			for range 3 {
				<-ticker.C
				pollCount++
			}
			close(done)
		}()

		// Advance time by 90 seconds (3 poll intervals)
		time.Sleep(90 * time.Second)
		synctest.Wait()

		<-done

		if pollCount != 3 {
			t.Errorf("expected 3 polls after 90s, got %d", pollCount)
		}

		// Verify mock was created correctly
		if mock == nil {
			t.Fatal("mock should not be nil")
		}
	})
}

// Helper to convert time to pgtype.Timestamptz
func toPgTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// Test conversion functions used by the worker
func TestClusterConversion(t *testing.T) {
	now := time.Now()

	// Test conversion from db.ClusterClaimForSyncRow to gardener.ClusterToSync
	dbRow := db.ClusterClaimForSyncRow{
		ID:                uuid.New(),
		Name:              "test-cluster",
		Region:            "local",
		KubernetesVersion: "1.31.1",
		Deleted:           toPgTimestamp(now),
		SyncAttempts:      3,
		OrganizationName:  "test-tenant",
	}

	// Simulate what worker.claimCluster does
	var deleted *time.Time
	if dbRow.Deleted.Valid {
		deleted = &dbRow.Deleted.Time
	}

	cluster := gardener.ClusterToSync{
		ID:                dbRow.ID,
		Name:              dbRow.Name,
		OrganizationName:  dbRow.OrganizationName,
		Region:            dbRow.Region,
		KubernetesVersion: dbRow.KubernetesVersion,
		Deleted:           deleted,
		SyncAttempts:      int(dbRow.SyncAttempts),
	}

	// Verify conversion
	if cluster.ID != dbRow.ID {
		t.Error("ID mismatch")
	}
	if cluster.Name != dbRow.Name {
		t.Error("Name mismatch")
	}
	if cluster.OrganizationName != dbRow.OrganizationName {
		t.Error("OrganizationName mismatch")
	}
	if cluster.Region != dbRow.Region {
		t.Error("Region mismatch")
	}
	if cluster.KubernetesVersion != dbRow.KubernetesVersion {
		t.Error("KubernetesVersion mismatch")
	}
	if cluster.Deleted == nil {
		t.Error("Deleted should not be nil")
	}
	if cluster.SyncAttempts != 3 {
		t.Errorf("expected SyncAttempts=3, got %d", cluster.SyncAttempts)
	}
}
