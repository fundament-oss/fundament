package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/common"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

var (
	_ handler.SyncHandler      = (*Handler)(nil)
	_ handler.StatusHandler    = (*Handler)(nil)
	_ handler.ReconcileHandler = (*Handler)(nil)
)

// mockDBTX implements db.DBTX for unit testing the handler without a real database.
type mockDBTX struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return nil, pgx.ErrNoRows
}

func (m *mockDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &noRow{}
}

// noRow returns ErrNoRows for any Scan call.
type noRow struct{}

func (r *noRow) Scan(_ ...any) error { return pgx.ErrNoRows }

// successRow returns nil for Scan, filling in a UUID for event-insert queries.
type successRow struct{}

func (r *successRow) Scan(dest ...any) error {
	if len(dest) > 0 {
		if p, ok := dest[0].(*uuid.UUID); ok {
			*p = uuid.New()
		}
	}
	return nil
}

func newTestHandler(mock *mockDBTX, gardenerClient gardener.Client) *Handler {
	logger := common.TestLogger()
	return &Handler{
		queries:  db.New(mock),
		gardener: gardenerClient,
		logger:   logger.With("handler", "cluster"),
		cfg:      Config{StatusBatchSize: 50},
	}
}

func TestSync_ClusterNotFound(t *testing.T) {
	mock := &mockDBTX{} // default queryRowFn returns noRow (ErrNoRows)
	gardenerMock := gardener.NewMockInstant(common.TestLogger())
	h := newTestHandler(mock, gardenerMock)

	err := h.Sync(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("expected nil error for not-found cluster, got: %v", err)
	}
}

func TestSync_CreateSuccess(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()

	mock := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			// ClusterGetForSync returns a valid row
			if containsSQL(sql, "clusters.region") {
				return &clusterGetForSyncRow{
					id: clusterID, name: "test", region: "local",
					k8sVersion: "1.31.1", orgID: orgID, orgName: "test-org",
				}
			}
			// Event insert queries return a UUID
			return &successRow{}
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	gardenerMock := gardener.NewMockInstant(common.TestLogger())
	h := newTestHandler(mock, gardenerMock)

	err := h.Sync(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gardenerMock.ApplyCalls) != 1 {
		t.Errorf("expected 1 ApplyShoot call, got %d", len(gardenerMock.ApplyCalls))
	}
	if gardenerMock.ApplyCalls[0].ID != clusterID {
		t.Error("ApplyShoot called with wrong cluster ID")
	}
}

func TestSync_DeleteSuccess(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()

	mock := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if containsSQL(sql, "clusters.region") {
				return &clusterGetForSyncDeletedRow{
					id: clusterID, name: "test", region: "local",
					k8sVersion: "1.31.1", orgID: orgID, orgName: "test-org",
				}
			}
			return &successRow{}
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	gardenerMock := gardener.NewMockInstant(common.TestLogger())
	// Pre-create a shoot so DeleteShootByClusterID succeeds
	tc := common.TestCluster("test", "test-org")
	tc.ID = clusterID
	_ = gardenerMock.ApplyShoot(context.Background(), &tc)

	h := newTestHandler(mock, gardenerMock)

	err := h.Sync(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gardenerMock.DeleteByClusterID) != 1 {
		t.Errorf("expected 1 DeleteShootByClusterID call, got %d", len(gardenerMock.DeleteByClusterID))
	}
}

func TestSync_EnsureProjectFailure(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()

	mock := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if containsSQL(sql, "clusters.region") {
				return &clusterGetForSyncRow{
					id: clusterID, name: "test", region: "local",
					k8sVersion: "1.31.1", orgID: orgID, orgName: "test-org",
				}
			}
			return &successRow{}
		},
	}

	gardenerMock := gardener.NewMockInstant(common.TestLogger())
	gardenerMock.EnsureProjectError = errors.New("project creation failed")
	h := newTestHandler(mock, gardenerMock)

	err := h.Sync(context.Background(), clusterID)
	if err == nil {
		t.Fatal("expected error from EnsureProject failure")
	}
	if !containsSQL(err.Error(), "ensure project") {
		t.Errorf("expected error to contain 'ensure project', got: %v", err)
	}
}

func TestSync_ApplyShootFailure(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()

	mock := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if containsSQL(sql, "clusters.region") {
				return &clusterGetForSyncRow{
					id: clusterID, name: "test", region: "local",
					k8sVersion: "1.31.1", orgID: orgID, orgName: "test-org",
				}
			}
			return &successRow{}
		},
	}

	gardenerMock := gardener.NewMockInstant(common.TestLogger())
	gardenerMock.SetApplyError(gardener.ErrMockApplyFailed)
	h := newTestHandler(mock, gardenerMock)

	err := h.Sync(context.Background(), clusterID)
	if err == nil {
		t.Fatal("expected error from ApplyShoot failure")
	}
	if !containsSQL(err.Error(), "apply shoot") {
		t.Errorf("expected error to contain 'apply shoot', got: %v", err)
	}
}

func TestSync_DeleteSkipsEnsureProject(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()

	mock := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if containsSQL(sql, "clusters.region") {
				return &clusterGetForSyncDeletedRow{
					id: clusterID, name: "test", region: "local",
					k8sVersion: "1.31.1", orgID: orgID, orgName: "test-org",
				}
			}
			return &successRow{}
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	gardenerMock := gardener.NewMockInstant(common.TestLogger())
	// Pre-create a shoot
	tc := common.TestCluster("test", "test-org")
	tc.ID = clusterID
	_ = gardenerMock.ApplyShoot(context.Background(), &tc)

	h := newTestHandler(mock, gardenerMock)

	err := h.Sync(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// EnsureProject should NOT have been called by the delete handler (D3: skip for deletes).
	// ApplyShoot in setup doesn't call EnsureProject, so total should be 0.
	if len(gardenerMock.EnsureProjectCalls) != 0 {
		t.Errorf("expected 0 EnsureProject calls (delete skips it), got %d", len(gardenerMock.EnsureProjectCalls))
	}
}

func TestReconcile_DriftDetection(t *testing.T) {
	clusterID := uuid.New()

	queryCallCount := 0
	mock := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			queryCallCount++
			if containsSQL(sql, "deleted IS NULL") {
				// ClusterListActive returns one cluster
				return &singleClusterRows{id: clusterID, name: "test"}, nil
			}
			return &emptyRows{}, nil
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			// OutboxInsertReconcile
			return pgconn.CommandTag{}, nil
		},
	}

	// Gardener has no shoots — drift expected
	gardenerMock := gardener.NewMockInstant(common.TestLogger())
	h := newTestHandler(mock, gardenerMock)

	err := h.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcile_OrphanCleanup(t *testing.T) {
	gardenerMock := gardener.NewMockInstant(common.TestLogger())

	// Create an orphan shoot in Gardener (no matching DB cluster)
	orphanCluster := common.TestCluster("orphan", "org")
	_ = gardenerMock.ApplyShoot(context.Background(), &orphanCluster)

	mock := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			// DB returns no active clusters
			return &emptyRows{}, nil
		},
	}

	h := newTestHandler(mock, gardenerMock)

	err := h.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Orphan should be deleted
	if len(gardenerMock.DeleteByClusterID) != 1 {
		t.Errorf("expected 1 delete call for orphan, got %d", len(gardenerMock.DeleteByClusterID))
	}
}

func TestCheckStatus_DeletedClusterConfirmed(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()

	var statusEventCreated bool
	var createdEventType string

	mock := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			// Check deleted verification first — both queries contain "shoot_status IS NULL"
			if containsSQL(sql, "deleted IS NOT NULL") {
				// ClusterListDeletedNeedingVerification — one deleted cluster
				return &deletedVerificationRows{
					id: clusterID, name: "test", region: "local",
					k8sVersion: "1.31.1", orgID: orgID, orgName: "test-org",
				}, nil
			}
			return &emptyRows{}, nil
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			if containsSQL(sql, "cluster_events") {
				statusEventCreated = true
				if len(args) >= 2 {
					if et, ok := args[1].(string); ok {
						createdEventType = et
					}
				}
			}
			return &successRow{}
		},
	}

	// Gardener has NO shoot for this cluster — GetShootStatus returns "pending"/"Shoot not found"
	gardenerMock := gardener.NewMockInstant(common.TestLogger())

	h := newTestHandler(mock, gardenerMock)

	err := h.CheckStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !statusEventCreated {
		t.Error("expected a status_deleted event for confirmed shoot deletion")
	}
	if createdEventType != "status_deleted" {
		t.Errorf("expected event_type 'status_deleted', got %q", createdEventType)
	}
}

// --- Test helpers ---

func containsSQL(s, substr string) bool {
	return len(s) >= len(substr) && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// clusterGetForSyncRow implements pgx.Row for a non-deleted cluster.
type clusterGetForSyncRow struct {
	id         uuid.UUID
	name       string
	region     string
	k8sVersion string
	orgID      uuid.UUID
	orgName    string
}

func (r *clusterGetForSyncRow) Scan(dest ...any) error {
	if len(dest) != 7 {
		return errors.New("wrong number of scan destinations")
	}
	*dest[0].(*uuid.UUID) = r.id
	*dest[1].(*string) = r.name
	*dest[2].(*string) = r.region
	*dest[3].(*string) = r.k8sVersion
	// dest[4] is pgtype.Timestamptz (Deleted) — leave zero value (not valid = not deleted)
	*dest[5].(*uuid.UUID) = r.orgID
	*dest[6].(*string) = r.orgName
	return nil
}

// clusterGetForSyncDeletedRow implements pgx.Row for a deleted cluster.
type clusterGetForSyncDeletedRow struct {
	id         uuid.UUID
	name       string
	region     string
	k8sVersion string
	orgID      uuid.UUID
	orgName    string
}

func (r *clusterGetForSyncDeletedRow) Scan(dest ...any) error {
	if len(dest) != 7 {
		return errors.New("wrong number of scan destinations")
	}
	*dest[0].(*uuid.UUID) = r.id
	*dest[1].(*string) = r.name
	*dest[2].(*string) = r.region
	*dest[3].(*string) = r.k8sVersion
	// Set Deleted to a valid timestamptz
	if ts, ok := dest[4].(*pgtype.Timestamptz); ok {
		ts.Valid = true
	}
	*dest[5].(*uuid.UUID) = r.orgID
	*dest[6].(*string) = r.orgName
	return nil
}

// emptyRows implements pgx.Rows returning no rows.
type emptyRows struct{ closed bool }

func (r *emptyRows) Close()                                       { r.closed = true }
func (r *emptyRows) Err() error                                   { return nil }
func (r *emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *emptyRows) Next() bool                                   { return false }
func (r *emptyRows) Scan(_ ...any) error                          { return pgx.ErrNoRows }
func (r *emptyRows) Values() ([]any, error)                       { return nil, nil }
func (r *emptyRows) RawValues() [][]byte                          { return nil }
func (r *emptyRows) Conn() *pgx.Conn                              { return nil }

// singleClusterRows implements pgx.Rows returning one ClusterListActiveRow.
type singleClusterRows struct {
	id      uuid.UUID
	name    string
	yielded bool
	closed  bool
}

func (r *singleClusterRows) Close()                                       { r.closed = true }
func (r *singleClusterRows) Err() error                                   { return nil }
func (r *singleClusterRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *singleClusterRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *singleClusterRows) Values() ([]any, error)                       { return nil, nil }
func (r *singleClusterRows) RawValues() [][]byte                          { return nil }
func (r *singleClusterRows) Conn() *pgx.Conn                              { return nil }

func (r *singleClusterRows) Next() bool {
	if r.yielded {
		return false
	}
	r.yielded = true
	return true
}

func (r *singleClusterRows) Scan(dest ...any) error {
	// ClusterListActive columns: id, name, deleted, synced, organization_name
	if len(dest) != 5 {
		return errors.New("wrong number of scan destinations")
	}
	*dest[0].(*uuid.UUID) = r.id
	*dest[1].(*string) = r.name
	// dest[2] (deleted) and dest[3] (synced) stay zero-value (null)
	*dest[4].(*string) = "test-org"
	return nil
}

// deletedVerificationRows returns one row for ClusterListDeletedNeedingVerification (9 columns).
type deletedVerificationRows struct {
	id         uuid.UUID
	name       string
	region     string
	k8sVersion string
	orgID      uuid.UUID
	orgName    string
	yielded    bool
	closed     bool
}

func (r *deletedVerificationRows) Close()                                       { r.closed = true }
func (r *deletedVerificationRows) Err() error                                   { return nil }
func (r *deletedVerificationRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *deletedVerificationRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *deletedVerificationRows) Values() ([]any, error)                       { return nil, nil }
func (r *deletedVerificationRows) RawValues() [][]byte                          { return nil }
func (r *deletedVerificationRows) Conn() *pgx.Conn                              { return nil }

func (r *deletedVerificationRows) Next() bool {
	if r.yielded {
		return false
	}
	r.yielded = true
	return true
}

func (r *deletedVerificationRows) Scan(dest ...any) error {
	// Columns: id, name, region, kubernetes_version, deleted, shoot_status, organization_id, shoot_status_updated, organization_name
	if len(dest) != 9 {
		return errors.New("wrong number of scan destinations")
	}
	*dest[0].(*uuid.UUID) = r.id
	*dest[1].(*string) = r.name
	*dest[2].(*string) = r.region
	*dest[3].(*string) = r.k8sVersion
	// Set deleted to a valid timestamp
	if ts, ok := dest[4].(*pgtype.Timestamptz); ok {
		ts.Valid = true
	}
	// shoot_status stays zero (null) — not yet confirmed
	*dest[6].(*uuid.UUID) = r.orgID
	// dest[7] (shoot_status_updated) stays zero (null)
	*dest[8].(*string) = r.orgName
	return nil
}
