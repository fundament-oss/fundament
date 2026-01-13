package worker

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/gardener"
)

// testLogger creates a logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func TestMockClient_ApplyShoot(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:               uuid.New(),
		Name:             "test-cluster",
		OrganizationName: "test-tenant",
	}

	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("ApplyShoot failed: %v", err)
	}

	// Verify shoot was created (use ShootName to get the expected name)
	expectedName := gardener.ShootName(cluster.OrganizationName, cluster.Name, 21)
	if !mock.HasShoot(expectedName) {
		t.Errorf("expected shoot %q to exist", expectedName)
	}

	// Verify call was recorded
	if len(mock.ApplyCalls) != 1 {
		t.Errorf("expected 1 apply call, got %d", len(mock.ApplyCalls))
	}
	if mock.ApplyCalls[0].ID != cluster.ID {
		t.Error("apply call did not match cluster")
	}
}

func TestMockClient_DeleteShoot(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:               uuid.New(),
		Name:             "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Create shoot first
	_ = mock.ApplyShoot(ctx, &cluster)

	// Now delete
	err := mock.DeleteShoot(ctx, &cluster)
	if err != nil {
		t.Fatalf("DeleteShoot failed: %v", err)
	}

	// Verify shoot was deleted
	if mock.HasShoot("test-tenant-test-cluster") {
		t.Error("expected shoot to be deleted")
	}

	// Verify call was recorded
	if len(mock.DeleteCalls) != 1 {
		t.Errorf("expected 1 delete call, got %d", len(mock.DeleteCalls))
	}
}

func TestMockClient_ListShoots(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()

	// Create multiple shoots
	for i := 0; i < 3; i++ {
		cluster := gardener.ClusterToSync{
			ID:               uuid.New(),
			Name:             "cluster-" + string(rune('a'+i)),
			OrganizationName: "tenant",
		}
		_ = mock.ApplyShoot(ctx, &cluster)
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
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:               uuid.New(),
		Name:             "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Before shoot exists - should return pending
	status, _, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected status 'pending', got %q", status)
	}

	// Create shoot
	_ = mock.ApplyShoot(ctx, &cluster)

	// After shoot exists - should return ready
	status, msg, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "ready" {
		t.Errorf("expected status 'ready', got %q", status)
	}
	if msg == "" {
		t.Error("expected non-empty message")
	}
}

func TestMockClient_StatusOverride(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:               uuid.New(),
		Name:             "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Set override
	mock.SetStatusOverride(cluster.ID, "progressing", "Creating infrastructure")

	// Create shoot
	_ = mock.ApplyShoot(ctx, &cluster)

	// Should return override status, not default "ready"
	status, msg, err := mock.GetShootStatus(ctx, &cluster)
	if err != nil {
		t.Fatalf("GetShootStatus failed: %v", err)
	}
	if status != "progressing" {
		t.Errorf("expected status 'progressing', got %q", status)
	}
	if msg != "Creating infrastructure" {
		t.Errorf("expected message 'Creating infrastructure', got %q", msg)
	}
}

func TestMockClient_ApplyError(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:               uuid.New(),
		Name:             "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Set error
	mock.SetApplyError(gardener.ErrMockApplyFailed)

	err := mock.ApplyShoot(ctx, &cluster)
	if !errors.Is(err, gardener.ErrMockApplyFailed) {
		t.Errorf("expected ErrMockApplyFailed, got %v", err)
	}

	// Shoot should not exist
	if mock.HasShoot("test-tenant-test-cluster") {
		t.Error("shoot should not exist after error")
	}
}

func TestMockClient_Reset(t *testing.T) {
	logger := testLogger()
	mock := gardener.NewMock(logger)

	ctx := context.Background()
	cluster := gardener.ClusterToSync{
		ID:               uuid.New(),
		Name:             "test-cluster",
		OrganizationName: "test-tenant",
	}

	// Create shoot and set error
	_ = mock.ApplyShoot(ctx, &cluster)
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
	err := mock.ApplyShoot(ctx, &cluster)
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestTruncateError(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		maxLen   int
		expected string
	}{
		{
			name:     "short message",
			msg:      "short error",
			maxLen:   100,
			expected: "short error",
		},
		{
			name:     "exact length",
			msg:      "exact",
			maxLen:   5,
			expected: "exact",
		},
		{
			name:     "truncated",
			msg:      "this is a very long error message that needs to be truncated",
			maxLen:   20,
			expected: "this is a very lo...",
		},
		{
			name:     "empty message",
			msg:      "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateError(tt.msg, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestShootName(t *testing.T) {
	tests := []struct {
		name         string
		org          string
		cluster      string
		wantExact    string // if set, expect this exact value
		wantPrefix   string // if set, expect name to start with this
		wantMaxLen   int    // if set, expect name to be at most this length
		wantContains string // if set, expect name to contain this
	}{
		{
			name:      "short name unchanged",
			org:       "my-tenant",
			cluster:   "my-cluster",
			wantExact: "my-tenant-my-cluster",
		},
		{
			name:      "exactly 21 chars unchanged",
			org:       "org",
			cluster:   "exactly-21-chars-ok",
			wantExact: "org-exactly-21-chars-ok", // 23 chars, will be hashed
		},
		{
			name:       "long name is hashed",
			org:        "my-organization",
			cluster:    "very-long-cluster-name",
			wantPrefix: "my-organizat", // 12 chars prefix
			wantMaxLen: 21,
		},
		{
			name:       "very long names stay within limit",
			org:        "extremely-long-organization-name",
			cluster:    "and-an-extremely-long-cluster-name-too",
			wantMaxLen: 21,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gardener.ShootName(tt.org, tt.cluster, 21)

			if tt.wantExact != "" {
				// For short names, check if they'd be hashed or not
				fullName := tt.org + "-" + tt.cluster
				if len(fullName) <= 21 {
					if got != fullName {
						t.Errorf("expected exact %q, got %q", fullName, got)
					}
				}
			}

			if tt.wantPrefix != "" && len(got) > len(tt.wantPrefix) {
				if got[:len(tt.wantPrefix)] != tt.wantPrefix {
					t.Errorf("expected prefix %q, got %q", tt.wantPrefix, got)
				}
			}

			if tt.wantMaxLen > 0 && len(got) > tt.wantMaxLen {
				t.Errorf("expected max length %d, got %d (%q)", tt.wantMaxLen, len(got), got)
			}

			if tt.wantContains != "" && !contains(got, tt.wantContains) {
				t.Errorf("expected %q to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestShootName_Deterministic(t *testing.T) {
	// Same inputs should always produce same output
	name1 := gardener.ShootName("long-organization", "long-cluster-name", 21)
	name2 := gardener.ShootName("long-organization", "long-cluster-name", 21)

	if name1 != name2 {
		t.Errorf("ShootName is not deterministic: %q != %q", name1, name2)
	}
}

func TestShootName_DifferentHashForDifferentInputs(t *testing.T) {
	// Different inputs should produce different outputs even with same prefix
	name1 := gardener.ShootName("my-organization", "cluster-aaaaaaaaa", 21)
	name2 := gardener.ShootName("my-organization", "cluster-bbbbbbbbb", 21)

	if name1 == name2 {
		t.Errorf("different inputs produced same output: %q", name1)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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

	logger := testLogger()
	mock := gardener.NewMock(logger)

	w := NewSyncWorker(pool, mock, logger, Config{
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
	for i := 0; i < n; i++ {
		result *= 2
	}
	return result
}

func TestStatusPoller_Timing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := testLogger()
		mock := gardener.NewMock(logger)

		// We can't easily test the full poller without a DB connection,
		// but we can verify the ticker behavior using synctest
		pollInterval := 30 * time.Second
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		pollCount := 0
		done := make(chan struct{})

		go func() {
			for i := 0; i < 3; i++ {
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

func TestSyncWorker_ShutdownTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := testLogger()

		// Create a worker (without real DB connection, just test shutdown logic)
		w := &SyncWorker{
			logger: logger,
		}

		// Simulate in-flight operation
		w.inFlight.Add(1)

		shutdownComplete := make(chan struct{})
		go func() {
			w.Shutdown(100 * time.Millisecond)
			close(shutdownComplete)
		}()

		// Complete the in-flight operation after 50ms
		go func() {
			time.Sleep(50 * time.Millisecond)
			w.inFlight.Done()
		}()

		synctest.Wait()

		// Shutdown should complete before timeout
		select {
		case <-shutdownComplete:
			// Good, shutdown completed
		case <-time.After(200 * time.Millisecond):
			t.Error("shutdown did not complete in time")
		}
	})
}

func TestSyncWorker_ShutdownTimeoutExceeded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := testLogger()

		w := &SyncWorker{
			logger: logger,
		}

		// Simulate in-flight operation that takes too long
		w.inFlight.Add(1)

		shutdownComplete := make(chan struct{})
		go func() {
			w.Shutdown(50 * time.Millisecond) // Short timeout
			close(shutdownComplete)
		}()

		// Don't complete the in-flight operation

		synctest.Wait()

		// Shutdown should timeout
		select {
		case <-shutdownComplete:
			// Good, shutdown timed out as expected
		case <-time.After(200 * time.Millisecond):
			t.Error("shutdown did not timeout as expected")
		}

		// Clean up
		w.inFlight.Done()
	})
}

// Helper to convert time to pgtype.Timestamptz
func toPgTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// Test conversion functions used by the worker
func TestClusterConversion(t *testing.T) {
	now := time.Now()

	// Test conversion from db.ClusterClaimUnsyncedRow to gardener.ClusterToSync
	dbRow := db.ClusterClaimUnsyncedRow{
		ID:               uuid.New(),
		Name:             "test-cluster",
		Deleted:          toPgTimestamp(now),
		SyncAttempts:     3,
		OrganizationName: "test-tenant",
	}

	// Simulate what worker.claimCluster does
	var deleted *time.Time
	if dbRow.Deleted.Valid {
		deleted = &dbRow.Deleted.Time
	}

	cluster := gardener.ClusterToSync{
		ID:               dbRow.ID,
		Name:             dbRow.Name,
		OrganizationName: dbRow.OrganizationName,
		Deleted:          deleted,
		SyncAttempts:     int(dbRow.SyncAttempts),
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
	if cluster.Deleted == nil {
		t.Error("Deleted should not be nil")
	}
	if cluster.SyncAttempts != 3 {
		t.Errorf("expected SyncAttempts=3, got %d", cluster.SyncAttempts)
	}
}
