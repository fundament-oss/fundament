package cluster_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/projectrbac"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/kubename"
)

// projectRBACFixture is a ready cluster with one project and one synced
// namespace: an org admin who is also project admin, a project admin, and a
// project viewer.
type projectRBACFixture struct {
	db          *testDB
	mock        *shoot.MockShootAccess
	clusterID   uuid.UUID
	projectID   uuid.UUID
	namespace   string
	orgAdmin    uuid.UUID
	projAdmin   uuid.UUID
	projViewer  uuid.UUID
	userSyncCtx handler.SyncContext
}

func newProjectRBACFixture(t *testing.T, name string) *projectRBACFixture {
	t.Helper()
	db := createTestDB(t)
	mock := newMockShootAccess(t)

	clusterID := insertCluster(t, db, acmeCorpOrgID, name)
	makeClusterReady(t, db, clusterID)
	markOutboxCompleted(t, db, clusterID)

	orgAdmin := insertUser(t, db, name+"-org-admin")
	projAdmin := insertUser(t, db, name+"-proj-admin")
	projViewer := insertUser(t, db, name+"-proj-viewer")
	insertOrgUser(t, db, acmeCorpOrgID, orgAdmin, "admin", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projAdmin, "viewer", "accepted")
	insertOrgUser(t, db, acmeCorpOrgID, projViewer, "viewer", "accepted")

	projectName := name + "-proj"
	projectID := insertProjectWithMembersNamed(t, db, clusterID, projectName,
		projectMember{UserID: orgAdmin, Role: "admin"},
		projectMember{UserID: projAdmin, Role: "admin"},
		projectMember{UserID: projViewer, Role: "viewer"},
	)
	nsID := insertNamespace(t, db, projectID, "web")
	require.NoError(t, newNamespaceHandler(t, db, mock).Sync(t.Context(), nsID, nsSyncCtx))

	return &projectRBACFixture{
		db: db, mock: mock, clusterID: clusterID, projectID: projectID,
		namespace: kubename.GenerateNamespace(projectName, "web"),
		orgAdmin:  orgAdmin, projAdmin: projAdmin, projViewer: projViewer,
		userSyncCtx: handler.SyncContext{
			EntityType: handler.EntityProjectMember,
			Event:      dbconst.ClusterOutboxEvent_Updated,
			Source:     dbconst.ClusterOutboxSource_Trigger,
		},
	}
}

func (f *projectRBACFixture) binding(user uuid.UUID) *shoot.ResourceInfo {
	return f.mock.GetRoleBinding(f.clusterID, f.namespace, projectrbac.BindingName(user))
}

func TestProjectRBAC_NamespaceSyncBindsMembers(t *testing.T) {
	t.Parallel()
	f := newProjectRBACFixture(t, "prbac-ns")

	admin := f.binding(f.projAdmin)
	require.NotNil(t, admin, "project admin binding")
	assert.Equal(t, "admin", admin.RoleRef.Name)
	assert.Equal(t, projectrbac.Subjects(f.projAdmin), admin.Subjects)

	viewer := f.binding(f.projViewer)
	require.NotNil(t, viewer, "project viewer binding")
	assert.Equal(t, "view", viewer.RoleRef.Name)

	assert.Nil(t, f.binding(f.orgAdmin), "org admins hold cluster-admin and get no project binding")
}

func TestProjectRBAC_MembershipChangesConverge(t *testing.T) {
	t.Parallel()
	f := newProjectRBACFixture(t, "prbac-member")
	h := newUserSyncHandler(t, f.db, f.mock)

	// Viewer promoted to admin.
	_, err := f.db.adminPool.Exec(t.Context(),
		`UPDATE tenant.project_members SET role = 'admin' WHERE user_id = $1`, f.projViewer)
	require.NoError(t, err)
	require.NoError(t, h.Sync(t.Context(), getProjectMemberID(t, f.db, f.projViewer), f.userSyncCtx))
	assert.Equal(t, "admin", f.binding(f.projViewer).RoleRef.Name)

	// Removed from the project: the binding goes, the SA follows usersync's rules.
	pmID := getProjectMemberID(t, f.db, f.projViewer)
	_, err = f.db.adminPool.Exec(t.Context(),
		`UPDATE tenant.project_members SET deleted = now() WHERE id = $1`, pmID)
	require.NoError(t, err)
	require.NoError(t, h.Sync(t.Context(), pmID, f.userSyncCtx))
	assert.Nil(t, f.binding(f.projViewer))
	assert.NotNil(t, f.binding(f.projAdmin), "other members are untouched")
}

func TestProjectRBAC_ReconcileRepairsDrift(t *testing.T) {
	t.Parallel()
	f := newProjectRBACFixture(t, "prbac-reconcile")
	h := newUserSyncHandler(t, f.db, f.mock)
	ctx := t.Context()

	stranger := uuid.New()
	require.NoError(t, f.mock.DeleteRoleBinding(ctx, f.clusterID, f.namespace, projectrbac.BindingName(f.projViewer)))
	require.NoError(t, f.mock.EnsureRoleBinding(ctx, f.clusterID, f.namespace, projectrbac.BindingName(f.projAdmin),
		projectrbac.RoleRef("view"), projectrbac.Subjects(f.projAdmin), projectrbac.Labels(f.projAdmin)))
	require.NoError(t, f.mock.EnsureRoleBinding(ctx, f.clusterID, f.namespace, projectrbac.BindingName(stranger),
		projectrbac.RoleRef("admin"), projectrbac.Subjects(stranger), projectrbac.Labels(stranger)))

	require.NoError(t, h.Reconcile(ctx))

	require.NotNil(t, f.binding(f.projViewer), "missing binding recreated")
	assert.Equal(t, "admin", f.binding(f.projAdmin).RoleRef.Name, "wrong role corrected")
	assert.Nil(t, f.binding(stranger), "orphan removed")
}
