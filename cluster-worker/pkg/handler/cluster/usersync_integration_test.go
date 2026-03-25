package cluster_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	dbgen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/usersync"
)

func newUserSyncHandler(t *testing.T, db *testDB, mock *usersync.MockShootAccess) *usersync.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return usersync.New(db.workerPool, mock, logger)
}

func newMockShootAccess(t *testing.T) *usersync.MockShootAccess {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return usersync.NewMockShootAccess(logger)
}

// makeClusterReady sets a cluster to ready with shoot URL and CA data.
func makeClusterReady(t *testing.T, db *testDB, clusterID uuid.UUID) {
	t.Helper()
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters
		 SET shoot_status = 'ready',
		     shoot_api_server_url = 'https://mock-api.example.com',
		     shoot_ca_data = 'bW9jay1jYQ==',
		     shoot_status_updated = now() - interval '1 minute'
		 WHERE id = $1`,
		clusterID,
	)
	require.NoError(t, err)
}

// getOrgUserID returns the organizations_users.id for a user in an org.
func getOrgUserID(t *testing.T, db *testDB, orgID, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT id FROM tenant.organizations_users
		 WHERE organization_id = $1 AND user_id = $2 AND deleted IS NULL
		 ORDER BY id DESC LIMIT 1`,
		orgID, userID,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// getProjectMemberID returns the project_members.id for a user in a project.
func getProjectMemberID(t *testing.T, db *testDB, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT id FROM tenant.project_members
		 WHERE user_id = $1 AND deleted IS NULL
		 ORDER BY id DESC LIMIT 1`,
		userID,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// --- 10.6: Org admin added → SA + CRB ---

func TestSyncOrgAdminAdded(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-org-admin")
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	userID := insertUser(t, db, "Org Admin")
	insertOrgUser(t, db, acmeCorpOrgID, userID, "admin", "accepted")
	orgUserID := getOrgUserID(t, db, acmeCorpOrgID, userID)

	err := h.Sync(t.Context(), orgUserID, handler.SyncContext{
		EntityType: handler.EntityOrgUser,
		Event:      "created",
		Source:     "trigger",
	})
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, userID), "SA should exist on shoot")
	require.True(t, mock.HasCRB(clusterID, userID), "CRB should exist on shoot")
}

// --- 10.7: Project member added → SA only ---

func TestSyncProjectMemberAdded(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-proj-member")
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	userID := insertUser(t, db, "Project Member")
	projectAdminID := insertUser(t, db, "Proj Admin PM")
	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projectAdminID, "admin", "accepted")

	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)
	pmID := getProjectMemberID(t, db, userID)

	err := h.Sync(t.Context(), pmID, handler.SyncContext{
		EntityType: handler.EntityProjectMember,
		Event:      "created",
		Source:     "trigger",
	})
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, userID), "SA should exist on shoot")
	require.False(t, mock.HasCRB(clusterID, userID), "CRB should NOT exist for project member")
}

// --- 10.8: Admin removed, still project member → CRB deleted, SA kept ---

func TestSyncAdminRemovedStillProjectMember(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-admin-demote")
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	userID := insertUser(t, db, "Demoting Admin")
	projectAdminID := insertUser(t, db, "Proj Admin DA")

	// User starts as admin with SA + CRB on shoot.
	insertOrgUser(t, db, acmeCorpOrgID, userID, "admin", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projectAdminID, "admin", "accepted")
	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)

	// Pre-populate shoot with SA + CRB (as if previously synced as admin).
	orgUserID := getOrgUserID(t, db, acmeCorpOrgID, userID)
	err := h.Sync(t.Context(), orgUserID, handler.SyncContext{
		EntityType: handler.EntityOrgUser,
		Event:      "created",
		Source:     "trigger",
	})
	require.NoError(t, err)
	require.True(t, mock.HasSA(clusterID, userID))
	require.True(t, mock.HasCRB(clusterID, userID))

	// Soft-delete admin membership.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.organizations_users SET deleted = now()
		 WHERE organization_id = $1 AND user_id = $2 AND permission = 'admin' AND deleted IS NULL`,
		acmeCorpOrgID, userID,
	)
	require.NoError(t, err)

	// Re-add as viewer.
	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")
	newOrgUserID := getOrgUserID(t, db, acmeCorpOrgID, userID)

	// Sync the updated org user row.
	err = h.Sync(t.Context(), newOrgUserID, handler.SyncContext{
		EntityType: handler.EntityOrgUser,
		Event:      "updated",
		Source:     "trigger",
	})
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, userID), "SA should be kept (still project member)")
	require.False(t, mock.HasCRB(clusterID, userID), "CRB should be deleted (no longer admin)")
}

// --- Non-ready skip-path tests ---

func TestSyncOrgUserAllClustersNotReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	// Cluster exists but is not ready.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-not-ready")
	setShootStatus(t, db, clusterID, "progressing")
	markOutboxCompleted(t, db, clusterID)

	userID := insertUser(t, db, "Not Ready User")
	insertOrgUser(t, db, acmeCorpOrgID, userID, "admin", "accepted")
	orgUserID := getOrgUserID(t, db, acmeCorpOrgID, userID)

	err := h.Sync(t.Context(), orgUserID, handler.SyncContext{
		EntityType: handler.EntityOrgUser,
		Event:      "created",
		Source:     "trigger",
	})
	require.NoError(t, err, "should succeed without error even with no ready clusters")
	require.False(t, mock.HasSA(clusterID, userID), "no SA should be created on non-ready cluster")
}

func TestSyncProjectMemberClusterNotReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-pm-not-ready")
	setShootStatus(t, db, clusterID, "progressing")
	markOutboxCompleted(t, db, clusterID)

	userID := insertUser(t, db, "PM Not Ready")
	projectAdminID := insertUser(t, db, "Proj Admin NR")
	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projectAdminID, "admin", "accepted")
	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)
	pmID := getProjectMemberID(t, db, userID)

	err := h.Sync(t.Context(), pmID, handler.SyncContext{
		EntityType: handler.EntityProjectMember,
		Event:      "created",
		Source:     "trigger",
	})
	require.NoError(t, err, "should succeed without error for non-ready cluster")
	require.False(t, mock.HasSA(clusterID, userID), "no SA should be created on non-ready cluster")
}

// --- 10.5: Cluster ready → all users provisioned ---

func TestSyncClusterReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "sync-cluster-ready")
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	adminID := insertUser(t, db, "Ready Admin")
	memberID := insertUser(t, db, "Ready Member")
	projectAdminID := insertUser(t, db, "Proj Admin CR")

	insertOrgUser(t, db, acmeCorpOrgID, adminID, "admin", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, memberID, "viewer", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projectAdminID, "admin", "accepted")

	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: memberID, Role: "viewer"},
	)

	err := h.Sync(t.Context(), clusterID, handler.SyncContext{
		EntityType: handler.EntityCluster,
		Event:      "ready",
		Source:     "status",
	})
	require.NoError(t, err)

	// Admin should have SA + CRB.
	require.True(t, mock.HasSA(clusterID, adminID), "admin SA should exist")
	require.True(t, mock.HasCRB(clusterID, adminID), "admin CRB should exist")

	// Project admin should also have SA + CRB.
	require.True(t, mock.HasSA(clusterID, projectAdminID), "project admin SA should exist")
	require.True(t, mock.HasCRB(clusterID, projectAdminID), "project admin CRB should exist")

	// Project member should have SA but no CRB.
	require.True(t, mock.HasSA(clusterID, memberID), "member SA should exist")
	require.False(t, mock.HasCRB(clusterID, memberID), "member CRB should not exist")
}

// --- DB trigger tests ---

func TestTriggerOrgUserInsertCreatesOutboxRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	userID := insertUser(t, db, "Trigger Org User")
	insertOrgUser(t, db, acmeCorpOrgID, userID, "admin", "accepted")

	var count int
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox
		 WHERE organization_user_id IS NOT NULL`,
	).Scan(&count)
	require.NoError(t, err)
	require.Greater(t, count, 0, "org user trigger should create outbox row")
}

func TestTriggerProjectMemberInsertCreatesOutboxRow(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "trigger-pm")
	userID := insertUser(t, db, "Trigger PM User")
	projectAdminID := insertUser(t, db, "Trigger Proj Admin")
	insertOrgUser(t, db, acmeCorpOrgID, projectAdminID, "admin", "accepted")

	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)

	var count int
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox
		 WHERE project_member_id IS NOT NULL`,
	).Scan(&count)
	require.NoError(t, err)
	require.Greater(t, count, 0, "project member trigger should create outbox row")
}

// --- Outbox routing tests ---

func TestOutboxRowCanBeLocked(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	userID := insertUser(t, db, "Entity Detect User")
	insertOrgUser(t, db, acmeCorpOrgID, userID, "admin", "accepted")

	// Verify OutboxGetAndLock can read and lock rows with different FK columns set.
	row, err := queries.OutboxGetAndLock(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, row.ID, "should get a valid outbox row")
}

// --- Reconciliation integration tests ---

func TestReconcileMissingSACreated(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "reconcile-missing")
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	adminID := insertUser(t, db, "Reconcile Admin")
	memberID := insertUser(t, db, "Reconcile Member")
	projectAdminID := insertUser(t, db, "Reconcile ProjAdmin")

	insertOrgUser(t, db, acmeCorpOrgID, adminID, "admin", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, memberID, "viewer", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projectAdminID, "admin", "accepted")

	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: memberID, Role: "viewer"},
	)

	// Shoot is empty — reconcile should create everything from scratch.
	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, adminID), "admin SA should be created")
	require.True(t, mock.HasCRB(clusterID, adminID), "admin CRB should be created")
	require.True(t, mock.HasSA(clusterID, projectAdminID), "project admin SA should be created")
	require.True(t, mock.HasCRB(clusterID, projectAdminID), "project admin CRB should be created")
	require.True(t, mock.HasSA(clusterID, memberID), "member SA should be created")
	require.False(t, mock.HasCRB(clusterID, memberID), "member should not have CRB")
}

func TestReconcileOrphanedSADeleted(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "reconcile-orphan")
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	// No users should have access, but an orphaned SA+CRB exists on the shoot.
	orphanUserID := uuid.New()
	_ = mock.EnsureNamespace(t.Context(), clusterID, usersync.FundamentNamespace)
	_ = mock.EnsureServiceAccount(t.Context(), clusterID, usersync.FundamentNamespace,
		usersync.SAName(orphanUserID),
		map[string]string{usersync.LabelUserID: orphanUserID.String()},
		map[string]string{usersync.AnnotationUserName: "orphan@example.com"},
	)
	_ = mock.EnsureClusterRoleBinding(t.Context(), clusterID,
		usersync.CRBName(orphanUserID), usersync.FundamentNamespace, usersync.SAName(orphanUserID),
		map[string]string{usersync.LabelUserID: orphanUserID.String()},
		map[string]string{usersync.AnnotationUserName: "orphan@example.com"},
	)

	require.True(t, mock.HasSA(clusterID, orphanUserID), "precondition: orphan SA exists")
	require.True(t, mock.HasCRB(clusterID, orphanUserID), "precondition: orphan CRB exists")

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	require.False(t, mock.HasSA(clusterID, orphanUserID), "orphaned SA should be deleted")
	require.False(t, mock.HasCRB(clusterID, orphanUserID), "orphaned CRB should be deleted")
}

func TestReconcileCRBMismatchFixed(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newUserSyncHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "reconcile-crb-fix")
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	// User was admin, got demoted to member (viewer in org, still project member).
	userID := insertUser(t, db, "CRB Mismatch User")
	projectAdminID := insertUser(t, db, "CRB Fix ProjAdmin")

	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projectAdminID, "admin", "accepted")
	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)

	// Stale state on shoot: user still has CRB from when they were admin.
	_ = mock.EnsureNamespace(t.Context(), clusterID, usersync.FundamentNamespace)
	_ = mock.EnsureServiceAccount(t.Context(), clusterID, usersync.FundamentNamespace,
		usersync.SAName(userID),
		map[string]string{usersync.LabelUserID: userID.String()},
		map[string]string{usersync.AnnotationUserName: "user@example.com"},
	)
	_ = mock.EnsureClusterRoleBinding(t.Context(), clusterID,
		usersync.CRBName(userID), usersync.FundamentNamespace, usersync.SAName(userID),
		map[string]string{usersync.LabelUserID: userID.String()},
		map[string]string{usersync.AnnotationUserName: "user@example.com"},
	)

	require.True(t, mock.HasCRB(clusterID, userID), "precondition: stale CRB exists")

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, userID), "SA should be kept (still project member)")
	require.False(t, mock.HasCRB(clusterID, userID), "stale CRB should be deleted (no longer admin)")
}
