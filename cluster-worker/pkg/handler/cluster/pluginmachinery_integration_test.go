package cluster_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/pluginmachinery"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/pluginmachinery/manifests"
)

const testControllerImage = "ghcr.io/fundament-oss/fundament/plugin-controller:test"

func newPluginMachineryHandler(t *testing.T, db *testDB, shootAccess shoot.ShootAccess) *pluginmachinery.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := pluginmachinery.Config{
		ControllerImage: testControllerImage,
		CatalogAPIURL:   "https://api.fundament.example.com",
	}
	return pluginmachinery.New(db.workerPool, shootAccess, cfg, logger)
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
	assert.Equal(t, "https://api.fundament.example.com", env["MARKETPLACE_CATALOG_API_URL"])
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

// hookedShootAccess runs a callback before the first and last verbs of a
// provision batch, so a test can observe the context the batch passes down or
// change the world mid-pass.
type hookedShootAccess struct {
	*shoot.MockShootAccess
	before func(ctx context.Context, clusterID uuid.UUID)
}

// EnsureCRD is the first verb a provision issues.
func (h *hookedShootAccess) EnsureCRD(ctx context.Context, clusterID uuid.UUID, manifest []byte) error {
	h.before(ctx, clusterID)
	if err := h.MockShootAccess.EnsureCRD(ctx, clusterID, manifest); err != nil {
		return fmt.Errorf("mock EnsureCRD: %w", err)
	}
	return nil
}

// EnsureDeployment is the last verb a provision issues.
func (h *hookedShootAccess) EnsureDeployment(ctx context.Context, clusterID uuid.UUID, deployment *appsv1.Deployment) error {
	h.before(ctx, clusterID)
	if err := h.MockShootAccess.EnsureDeployment(ctx, clusterID, deployment); err != nil {
		return fmt.Errorf("mock EnsureDeployment: %w", err)
	}
	return nil
}

// Provisioning walks six resources on the shoot, and an uncached shoot client
// costs a Garden shoot lookup plus a CA-signed certificate to build. The whole
// batch must therefore share one credential set — checked at both ends, so
// neither dropping the opt-in nor moving it later in provision goes unnoticed.
func TestPluginMachineryProvisionSharesOneCredentialSet(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)

	var probed, uncached int
	access := &hookedShootAccess{MockShootAccess: mock}
	access.before = func(ctx context.Context, _ uuid.UUID) {
		probed++
		if !shoot.HasClientCache(ctx) {
			uncached++
		}
	}
	h := newPluginMachineryHandler(t, db, access)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "pm-cred-batch")
	makeClusterReady(t, db, clusterID)

	err := h.Sync(t.Context(), clusterID, handler.SyncContext{EntityType: handler.EntityCluster})
	require.NoError(t, err)

	require.Equal(t, 2, probed, "both ends of the provision batch must be observed")
	assert.Zero(t, uncached, "every shoot call in a provision must share one credential set")
}

// A cluster that stops being ready between ClusterListReady and its own
// re-read is a benign race, not a reconcile failure: reconcile errors feed the
// worker's consecutive-failure counter, which exits the process at three. The
// race is staged by flipping one cluster out of ready while the other is being
// provisioned, so the loop meets a row it listed as ready and now is not.
func TestPluginMachineryReconcileSkipsClusterThatLeavesReadyMidPass(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMockShootAccess(t)

	first := insertCluster(t, db, acmeCorpOrgID, "pm-race-first")
	makeClusterReady(t, db, first)
	second := insertCluster(t, db, acmeCorpOrgID, "pm-race-second")
	makeClusterReady(t, db, second)

	var provisioned, leftReady uuid.UUID
	access := &hookedShootAccess{MockShootAccess: mock}
	access.before = func(_ context.Context, clusterID uuid.UUID) {
		if provisioned != uuid.Nil {
			return
		}
		provisioned = clusterID
		leftReady = first
		if clusterID == first {
			leftReady = second
		}
		setShootStatus(t, db, leftReady, string(gardener.StatusProgressing))
	}
	h := newPluginMachineryHandler(t, db, access)

	err := h.Reconcile(t.Context())
	require.NoError(t, err, "a shoot leaving ready mid-pass is not a reconcile failure")

	require.NotNil(t, shootDeployment(mock, provisioned))
	assert.Nil(t, shootDeployment(mock, leftReady), "the cluster that left ready must be skipped, not provisioned")
}
