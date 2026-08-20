package cluster_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/pluginmachinery"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/pluginmachinery/manifests"
)

const testControllerImage = "ghcr.io/fundament-oss/fundament/plugin-controller:test"

func newPluginMachineryHandler(t *testing.T, db *testDB, mock *shoot.MockShootAccess) *pluginmachinery.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := pluginmachinery.Config{
		ControllerImage:    testControllerImage,
		OrganizationAPIURL: "https://api.fundament.example.com",
	}
	return pluginmachinery.New(db.workerPool, mock, cfg, logger)
}

func shootDeployment(mock *shoot.MockShootAccess, clusterID uuid.UUID) *appsv1.Deployment {
	return mock.Deployments[clusterID][manifests.Namespace+"/"+manifests.DeploymentName]
}

// The cluster-ready event provisions the full machinery: CRD, namespace, SA,
// ClusterRole, CRB and a Deployment stamped with the cluster's real identity.
func TestPluginMachinerySyncProvisionsEverything(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newPluginMachineryHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "pm-provision")
	makeClusterReady(t, db, clusterID)

	err := h.Sync(t.Context(), clusterID, handler.SyncContext{EntityType: handler.EntityCluster})
	require.NoError(t, err)

	// CRD
	require.Contains(t, mock.CRDs[clusterID], "plugininstallations.plugins.fundament.io")

	// Namespace + SA
	require.Contains(t, mock.Namespaces[clusterID], manifests.Namespace)
	require.Contains(t, mock.ServiceAccounts[clusterID][manifests.Namespace], manifests.DeploymentName)

	// ClusterRole with the chart-mirrored rules, CRB bound to it (not cluster-admin)
	role, ok := mock.ClusterRoles[clusterID][manifests.ClusterRoleName]
	require.True(t, ok)
	assert.Equal(t, manifests.ClusterRoleRules(), role.Rules)
	crb, ok := mock.ClusterRoleBindings[clusterID][manifests.ClusterRoleName]
	require.True(t, ok)
	assert.Equal(t, manifests.ClusterRoleName, crb.RoleRef.Name)

	// Deployment with real per-shoot identity
	d := shootDeployment(mock, clusterID)
	require.NotNil(t, d)
	env := map[string]string{}
	for _, e := range d.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	assert.Equal(t, testControllerImage, d.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, clusterID.String(), env["FUNDAMENT_CLUSTER_ID"])
	assert.Equal(t, acmeCorpOrgID.String(), env["FUNDAMENT_ORGANIZATION_ID"])
	assert.Equal(t, "https://api.fundament.example.com", env["ORGANIZATION_API_URL"])
	assert.NotContains(t, env, "PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH")
}

// Reconcile provisions every ready cluster and heals hand-deleted resources.
func TestPluginMachineryReconcileHeals(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newPluginMachineryHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "pm-heal")
	makeClusterReady(t, db, clusterID)

	err := h.Reconcile(t.Context())
	require.NoError(t, err)
	require.NotNil(t, shootDeployment(mock, clusterID))

	// Someone deletes the Deployment on the shoot; the next reconcile restores it.
	delete(mock.Deployments[clusterID], manifests.Namespace+"/"+manifests.DeploymentName)
	err = h.Reconcile(t.Context())
	require.NoError(t, err)
	require.NotNil(t, shootDeployment(mock, clusterID))
}

// Clusters that are not ready (or soft-deleted) are not provisioned by the
// reconcile loop.
func TestPluginMachineryReconcileSkipsNotReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newPluginMachineryHandler(t, db, mock)

	notReady := insertCluster(t, db, acmeCorpOrgID, "pm-not-ready")
	deleted := insertDeletedCluster(t, db, acmeCorpOrgID, "pm-deleted")

	err := h.Reconcile(t.Context())
	require.NoError(t, err)

	assert.Nil(t, shootDeployment(mock, notReady))
	assert.Nil(t, shootDeployment(mock, deleted))
}

// A stray ready event for a cluster whose shoot is not (or no longer) ready
// defers with a PreconditionError instead of failing hard — the outbox worker
// re-queues it without burning retries, and the reconcile loop heals later.
func TestPluginMachinerySyncDefersWhenNotReady(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newPluginMachineryHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "pm-not-ready-event")

	err := h.Sync(t.Context(), clusterID, handler.SyncContext{EntityType: handler.EntityCluster})
	var precond *handler.PreconditionError
	require.ErrorAs(t, err, &precond)
	assert.Nil(t, shootDeployment(mock, clusterID))
}

// A soft-deleted cluster that still gets a stray ready event is skipped
// gracefully (the org lookup filters deleted rows).
func TestPluginMachinerySyncSkipsDeletedCluster(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	h := newPluginMachineryHandler(t, db, mock)

	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "pm-stray-event")

	err := h.Sync(t.Context(), clusterID, handler.SyncContext{EntityType: handler.EntityCluster})
	require.NoError(t, err)
	assert.Nil(t, shootDeployment(mock, clusterID))
}

// Shoot-side failures surface as reconcile errors instead of being swallowed,
// so the reconcile loop's logging/metrics see them.
func TestPluginMachineryReconcileSurfacesErrors(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)
	mock.EnsureCRDError = assert.AnError
	h := newPluginMachineryHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "pm-error")
	makeClusterReady(t, db, clusterID)

	err := h.Reconcile(t.Context())
	require.Error(t, err)
}
