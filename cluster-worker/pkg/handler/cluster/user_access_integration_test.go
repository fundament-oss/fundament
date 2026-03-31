package cluster_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	dbgen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

func TestResolveUserAccessIgnoresDeletedOrgMembershipHistory(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.adminPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "access-org-history")
	userID := insertUser(t, db, "Org History User")

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status, deleted)
		 VALUES ($1, $2, 'admin', 'accepted', now())`,
		acmeCorpOrgID, userID,
	)
	require.NoError(t, err)

	_, err = db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status)
		 VALUES ($1, $2, 'admin', 'accepted')`,
		acmeCorpOrgID, userID,
	)
	require.NoError(t, err)

	access, err := queries.ResolveUserAccess(t.Context(), dbgen.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	require.NoError(t, err)
	require.Equal(t, "admin", access)
}

func TestResolveUserAccessIgnoresDeletedProjectMembershipHistory(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.adminPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "access-project-history")
	projectAdminID := insertUser(t, db, "Project Admin")
	userID := insertUser(t, db, "Project History User")

	projectID := insertProjectWithMembers(t, db, clusterID,
		projectMember{
			UserID: projectAdminID,
			Role:   "admin",
		},
		projectMember{
			UserID:  userID,
			Role:    "viewer",
			Deleted: true,
		},
		projectMember{
			UserID: userID,
			Role:   "viewer",
		},
	)
	require.NotEqual(t, uuid.Nil, projectID)

	access, err := queries.ResolveUserAccess(t.Context(), dbgen.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	require.NoError(t, err)
	require.Equal(t, "member", access)
}

func TestResolveUserAccessAdmin(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.adminPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "access-admin")
	userID := insertUser(t, db, "Admin User")
	insertOrgUser(t, db, acmeCorpOrgID, userID, "admin", "accepted")

	access, err := queries.ResolveUserAccess(t.Context(), dbgen.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	require.NoError(t, err)
	require.Equal(t, "admin", access)
}

func TestResolveUserAccessMember(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.adminPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "access-member")
	userID := insertUser(t, db, "Member User")
	projectAdminID := insertUser(t, db, "Project Admin M")

	// User is an org viewer (not admin), but a project member.
	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")
	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)

	access, err := queries.ResolveUserAccess(t.Context(), dbgen.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	require.NoError(t, err)
	require.Equal(t, "member", access)
}

func TestResolveUserAccessNone(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.adminPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "access-none")
	userID := insertUser(t, db, "No Access User")

	// User is in the org as viewer but not a project member.
	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")

	access, err := queries.ResolveUserAccess(t.Context(), dbgen.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	require.NoError(t, err)
	require.Equal(t, "none", access)
}

func TestResolveUserAccessAdminDemotedButProjectMember(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.adminPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "access-demoted")
	userID := insertUser(t, db, "Demoted User")
	projectAdminID := insertUser(t, db, "Project Admin D")

	// User was admin but got soft-deleted, still a project member.
	insertOrgUser(t, db, acmeCorpOrgID, userID, "admin", "accepted")
	// Soft-delete the admin membership.
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.organizations_users SET deleted = now()
		 WHERE organization_id = $1 AND user_id = $2 AND deleted IS NULL`,
		acmeCorpOrgID, userID,
	)
	require.NoError(t, err)

	// Re-add as viewer.
	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")

	insertProjectWithMembers(t, db, clusterID,
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)

	access, err := queries.ResolveUserAccess(t.Context(), dbgen.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	require.NoError(t, err)
	require.Equal(t, "member", access)
}

func TestResolveUserAccessMultiProject(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.adminPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "access-multi-proj")
	userID := insertUser(t, db, "Multi Project User")
	projectAdminID := insertUser(t, db, "Project Admin MP")

	// User is a viewer in the org (not admin), member of two projects on the same cluster.
	insertOrgUser(t, db, acmeCorpOrgID, userID, "viewer", "accepted")

	insertProjectWithMembersNamed(t, db, clusterID, "proj-a",
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)
	insertProjectWithMembersNamed(t, db, clusterID, "proj-b",
		projectMember{UserID: projectAdminID, Role: "admin"},
		projectMember{UserID: userID, Role: "viewer"},
	)

	access, err := queries.ResolveUserAccess(t.Context(), dbgen.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	require.NoError(t, err)
	require.Equal(t, "member", access)
}

// insertOrgUser inserts an organizations_users row.
func insertOrgUser(t *testing.T, db *testDB, orgID, userID uuid.UUID, permission, status string) {
	t.Helper()

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status)
		 VALUES ($1, $2, $3, $4)`,
		orgID, userID, permission, status,
	)
	require.NoError(t, err)
}

// insertProjectWithMembersNamed is like insertProjectWithMembers but with a custom project name.
func insertProjectWithMembersNamed(t *testing.T, db *testDB, clusterID uuid.UUID, projectName string, members ...projectMember) uuid.UUID {
	t.Helper()

	tx, err := db.adminPool.Begin(context.Background())
	require.NoError(t, err)
	defer func() {
		if tx != nil {
			_ = tx.Rollback(context.Background())
		}
	}()

	var projectID uuid.UUID
	err = tx.QueryRow(context.Background(),
		`INSERT INTO tenant.projects (cluster_id, name)
		 VALUES ($1, $2)
		 RETURNING id`,
		clusterID, projectName,
	).Scan(&projectID)
	require.NoError(t, err)

	for _, member := range members {
		if member.Deleted {
			_, err = tx.Exec(context.Background(),
				`INSERT INTO tenant.project_members (project_id, user_id, role, deleted)
				 VALUES ($1, $2, $3, now())`,
				projectID, member.UserID, member.Role,
			)
		} else {
			_, err = tx.Exec(context.Background(),
				`INSERT INTO tenant.project_members (project_id, user_id, role)
				 VALUES ($1, $2, $3)`,
				projectID, member.UserID, member.Role,
			)
		}
		require.NoError(t, err)
	}

	err = tx.Commit(context.Background())
	require.NoError(t, err)
	tx = nil

	return projectID
}

func insertUser(t *testing.T, db *testDB, name string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`INSERT INTO tenant.users (name, email)
		 VALUES ($1, $2)
		 RETURNING id`,
		name, name+"@example.com",
	).Scan(&id)
	require.NoError(t, err)

	return id
}

type projectMember struct {
	UserID  uuid.UUID
	Role    string
	Deleted bool
}

func insertProjectWithMembers(t *testing.T, db *testDB, clusterID uuid.UUID, members ...projectMember) uuid.UUID {
	t.Helper()

	tx, err := db.adminPool.Begin(context.Background())
	require.NoError(t, err)
	defer func() {
		if tx != nil {
			_ = tx.Rollback(context.Background())
		}
	}()

	var projectID uuid.UUID
	err = tx.QueryRow(context.Background(),
		`INSERT INTO tenant.projects (cluster_id, name)
		 VALUES ($1, $2)
		 RETURNING id`,
		clusterID, "project-"+clusterID.String()[:8],
	).Scan(&projectID)
	require.NoError(t, err)

	for _, member := range members {
		if member.Deleted {
			_, err = tx.Exec(context.Background(),
				`INSERT INTO tenant.project_members (project_id, user_id, role, deleted)
				 VALUES ($1, $2, $3, now())`,
				projectID, member.UserID, member.Role,
			)
		} else {
			_, err = tx.Exec(context.Background(),
				`INSERT INTO tenant.project_members (project_id, user_id, role)
				 VALUES ($1, $2, $3)`,
				projectID, member.UserID, member.Role,
			)
		}
		require.NoError(t, err)
	}

	err = tx.Commit(context.Background())
	require.NoError(t, err)
	tx = nil

	return projectID
}
