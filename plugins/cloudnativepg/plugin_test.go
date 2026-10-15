package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	sdktesting "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/testing"
	"github.com/fundament-oss/fundament/plugins/cloudnativepg/cnpg"
)

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	plugin, err := newPlugin()
	require.NoError(t, err)

	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin

	// For Uninstall, which refuses while databases exist.
	var _ pluginruntime.Installer = plugin
}

// newFakeClient builds a client whose scheme mirrors newPlugin's, plus the CNPG
// kinds as unstructured types so the fake client can track them.
func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	plugin, err := newPlugin()
	require.NoError(t, err)
	gvk := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	plugin.scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	plugin.scheme.AddKnownTypeWithName(clusterListGVK, &unstructured.UnstructuredList{})
	return fake.NewClientBuilder().WithScheme(plugin.scheme).WithObjects(objs...).Build()
}

func cluster(name, phase string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "Cluster",
		"metadata":   map[string]any{"name": name, "namespace": "team-a"},
		"spec":       map[string]any{"instances": int64(1)},
	}}
	if phase != "" {
		u.Object["status"] = map[string]any{"phase": phase}
	}
	return u
}

func storageClass(name string, isDefault bool) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: name}, Provisioner: "rancher.io/local-path"}
	if isDefault {
		sc.Annotations = map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}
	}
	return sc
}

func ourCRD(name string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name:        name,
		Annotations: map[string]string{"meta.helm.sh/release-name": cnpg.Release, "meta.helm.sh/release-namespace": cnpg.Namespace},
	}}
}

// Functions, not shared values: the fake client sets the resourceVersion of
// the objects it is built with, and the tests run in parallel.
func clustersCRD() *apiextensionsv1.CustomResourceDefinition { return ourCRD(cnpg.CRDNames[0]) }

func catalogCRD() *apiextensionsv1.CustomResourceDefinition { return ourCRD(cnpg.CRDNames[1]) }

const testImage = "ghcr.io/cloudnative-pg/postgresql:17.11-202610120828-standard-bookworm@sha256:fe22"

func operatorDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "cnpg-cloudnative-pg",
		Namespace: cnpg.Namespace,
		Labels:    map[string]string{"app.kubernetes.io/name": cnpg.Chart, "app.kubernetes.io/instance": cnpg.Release},
	}}
}

// newTestPlugin returns a plugin whose install never shells out to helm: it
// fails the test unless the test replaces it.
func newTestPlugin(t *testing.T) *Plugin {
	t.Helper()
	plugin, err := newPlugin()
	require.NoError(t, err)
	plugin.install = func(context.Context, client.Client) error {
		t.Error("unexpected operator install")
		return nil
	}
	plugin.uninstallOperator = func(context.Context) error {
		t.Error("unexpected operator uninstall")
		return nil
	}
	plugin.catalog = []cnpg.CatalogImage{{Major: 17, Image: testImage}}
	return plugin
}

func TestReconcileReportsStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		objs    []client.Object
		message string
	}{
		{
			name:    "no databases",
			objs:    []client.Object{storageClass("local-path", true)},
			message: "CloudNativePG operator running; no databases yet",
		},
		{
			name: "healthy and not ready",
			objs: []client.Object{
				storageClass("local-path", true),
				cluster("orders", healthyPhase),
				cluster("billing", "Setting up primary"),
				cluster("fresh", ""),
			},
			message: "3 databases: 1 healthy, 2 not ready",
		},
		{
			name:    "no default StorageClass",
			objs:    []client.Object{storageClass("ceph-block", false), cluster("orders", healthyPhase)},
			message: "1 database: 1 healthy, 0 not ready; no default StorageClass, pick one when creating a database",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plugin := newTestPlugin(t)
			kube := newFakeClient(t, append(tt.objs, clustersCRD(), catalogCRD(), operatorDeployment())...)
			plugin.kube.Store(&kube)
			host := sdktesting.NewMockHost()

			require.NoError(t, plugin.Reconcile(t.Context(), host))
			assert.Equal(t, pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: tt.message}, host.CurrentStatus())
		})
	}
}

// A deleted operator Deployment is put back by Reconcile, not only on the next
// pod restart: Helm still says deployed, so nothing else would.
func TestReconcileReinstallsDeletedOperator(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	kube := newFakeClient(t, storageClass("local-path", true), clustersCRD(), catalogCRD())
	plugin.kube.Store(&kube)
	installs := 0
	plugin.install = func(ctx context.Context, kube client.Client) error {
		installs++
		return kube.Create(ctx, operatorDeployment())
	}
	host := sdktesting.NewMockHost()

	require.NoError(t, plugin.Reconcile(t.Context(), host))
	assert.Equal(t, 1, installs)
	require.NotEmpty(t, host.StatusHistory)
	assert.Equal(t, pluginruntime.PhaseInstalling, host.StatusHistory[0].Phase)
	assert.Equal(t, pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "CloudNativePG operator running; no databases yet"}, host.CurrentStatus())
}

func TestReconcileReinstallFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		err   error
		phase pluginruntime.PluginPhase
	}{
		{"helm error", fmt.Errorf("helm install failed"), pluginruntime.PhaseDegraded},
		// An earlier pod's install is still running.
		{"in progress", fmt.Errorf("%w: release is pending-upgrade", cnpg.ErrInProgress), pluginruntime.PhaseInstalling},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plugin := newTestPlugin(t)
			kube := newFakeClient(t, clustersCRD())
			plugin.kube.Store(&kube)
			plugin.install = func(context.Context, client.Client) error { return tt.err }
			host := sdktesting.NewMockHost()

			require.Error(t, plugin.Reconcile(t.Context(), host))
			assert.Equal(t, tt.phase, host.CurrentStatus().Phase)
		})
	}
}

func TestReconcileDegradesWithoutCRD(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	kube := newFakeClient(t, operatorDeployment())
	plugin.kube.Store(&kube)
	// The install does not bring the CRD back.
	plugin.install = func(context.Context, client.Client) error { return nil }
	host := sdktesting.NewMockHost()

	require.Error(t, plugin.Reconcile(t.Context(), host))
	assert.Equal(t, pluginruntime.PhaseDegraded, host.CurrentStatus().Phase)
}

// Reconcile runs on a ticker from the start; before Start has a client it is a
// no-op rather than a nil dereference.
func TestReconcileBeforeStart(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	host := sdktesting.NewMockHost()

	require.NoError(t, plugin.Reconcile(t.Context(), host))
	assert.Empty(t, host.StatusHistory)
}

func TestApplyUserRoles(t *testing.T) {
	t.Parallel()
	kube := newFakeClient(t)

	// Twice: Start runs on every restart.
	require.NoError(t, applyUserRoles(t.Context(), kube))
	require.NoError(t, applyUserRoles(t.Context(), kube))

	wantVerbs := map[string][]string{
		"fundament-cloudnativepg-admin": {"get", "list", "watch", "create", "delete"},
		"fundament-cloudnativepg-view":  {"get", "list", "watch"},
	}
	wantLabel := map[string]string{
		"fundament-cloudnativepg-admin": "rbac.authorization.k8s.io/aggregate-to-admin",
		"fundament-cloudnativepg-view":  "rbac.authorization.k8s.io/aggregate-to-view",
	}
	for name, verbs := range wantVerbs {
		var role rbacv1.ClusterRole
		require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: name}, &role))
		assert.Equal(t, "true", role.Labels[wantLabel[name]], name)
		require.Len(t, role.Rules, 1, name)
		assert.Equal(t, []string{"postgresql.cnpg.io"}, role.Rules[0].APIGroups, name)
		assert.Equal(t, []string{"clusters"}, role.Rules[0].Resources, name)
		assert.ElementsMatch(t, verbs, role.Rules[0].Verbs, name)
	}
}

// Project admins must not be able to update a Cluster, nor delete them in
// bulk, including through the console's generated views and kubectl. Only
// admins may delete one; viewers stay read-only.
func TestUserRolesGrantNoModification(t *testing.T) {
	t.Parallel()
	for _, role := range userRoles() {
		forbidden := []string{"update", "patch", "deletecollection", "*"}
		if role.Name != "fundament-cloudnativepg-admin" {
			forbidden = append(forbidden, "delete")
		}
		for _, rule := range role.Rules {
			for _, verb := range forbidden {
				assert.NotContains(t, rule.Verbs, verb, "%s must not grant %s", role.Name, verb)
			}
		}
	}
}

// An operator someone took over after Start is reported, not repaired or
// upgraded: every operator version change restarts every database.
func TestReconcileLeavesForeignOperator(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	openfscCRD := ourCRD(cnpg.CRDNames[0])
	openfscCRD.Annotations = map[string]string{"meta.helm.sh/release-name": "cnpg", "meta.helm.sh/release-namespace": "cnpg-system"}
	kube := newFakeClient(t, openfscCRD)
	plugin.kube.Store(&kube)
	host := sdktesting.NewMockHost()

	require.NoError(t, plugin.Reconcile(t.Context(), host))
	status := host.CurrentStatus()
	assert.Equal(t, pluginruntime.PhaseDegraded, status.Phase)
	assert.Contains(t, status.Message, "belongs to Helm release cnpg-system/cnpg")
}

// A deleted catalog comes back: databases referring to it do not start
// without it.
func TestReconcileRestoresCatalog(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	kube := newFakeClient(t, storageClass("local-path", true), clustersCRD(), catalogCRD(), operatorDeployment())
	plugin.kube.Store(&kube)

	require.NoError(t, plugin.Reconcile(t.Context(), sdktesting.NewMockHost()))
	catalog := &unstructured.Unstructured{}
	catalog.SetGroupVersionKind(cnpg.CatalogGVK)
	require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: cnpg.CatalogName}, catalog))
}

func TestSetUp(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	kube := newFakeClient(t)
	plugin.install = func(ctx context.Context, kube client.Client) error {
		for _, crd := range []client.Object{ourCRD(cnpg.CRDNames[0]), ourCRD(cnpg.CRDNames[1])} {
			if err := kube.Create(ctx, crd); err != nil {
				return err
			}
		}
		return nil
	}

	require.NoError(t, plugin.setUp(t.Context(), kube))
	catalog := &unstructured.Unstructured{}
	catalog.SetGroupVersionKind(cnpg.CatalogGVK)
	require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: cnpg.CatalogName}, catalog))
	var role rbacv1.ClusterRole
	require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: "fundament-cloudnativepg-admin"}, &role))
}

func TestSetUpFailurePhase(t *testing.T) {
	t.Parallel()
	assert.Equal(t, pluginruntime.PhaseFailed, setUpFailurePhase(fmt.Errorf("install operator: %w: CRD x belongs to Helm release a/b", cnpg.ErrForeignOperator)))
	assert.Equal(t, pluginruntime.PhaseInstalling, setUpFailurePhase(fmt.Errorf("install operator: %w", cnpg.ErrInProgress)))
	assert.Equal(t, pluginruntime.PhaseDegraded, setUpFailurePhase(fmt.Errorf("helm install failed")))
}

func TestUninstallRefusesWithDatabases(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	kube := newFakeClient(t, clustersCRD(), cluster("orders", healthyPhase))
	require.NoError(t, applyUserRoles(t.Context(), kube))
	plugin.kube.Store(&kube)
	host := sdktesting.NewMockHost()

	err := plugin.Uninstall(t.Context(), host)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team-a/orders")
	assert.Equal(t, pluginruntime.PhaseDegraded, host.CurrentStatus().Phase)
	// Project admins keep managing their databases.
	var role rbacv1.ClusterRole
	require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: "fundament-cloudnativepg-admin"}, &role))
}

func TestUninstall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		crd           *apiextensionsv1.CustomResourceDefinition
		wantUninstall bool
	}{
		{"ours", clustersCRD(), true},
		{"someone else's", func() *apiextensionsv1.CustomResourceDefinition {
			crd := ourCRD(cnpg.CRDNames[0])
			crd.Annotations = nil
			return crd
		}(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plugin := newTestPlugin(t)
			kube := newFakeClient(t, tt.crd)
			require.NoError(t, applyUserRoles(t.Context(), kube))
			require.NoError(t, cnpg.ApplyCatalog(t.Context(), kube, plugin.catalog))
			plugin.kube.Store(&kube)
			uninstalls := 0
			plugin.uninstallOperator = func(context.Context) error {
				uninstalls++
				return nil
			}

			require.NoError(t, plugin.Uninstall(t.Context(), sdktesting.NewMockHost()))
			assert.Equal(t, tt.wantUninstall, uninstalls == 1)
			for _, role := range userRoles() {
				err := kube.Get(t.Context(), client.ObjectKey{Name: role.Name}, &rbacv1.ClusterRole{})
				assert.True(t, apierrors.IsNotFound(err), role.Name)
			}
			catalog := &unstructured.Unstructured{}
			catalog.SetGroupVersionKind(cnpg.CatalogGVK)
			assert.True(t, apierrors.IsNotFound(kube.Get(t.Context(), client.ObjectKey{Name: cnpg.CatalogName}, catalog)))
		})
	}
}

func onCatalog(u *unstructured.Unstructured) *unstructured.Unstructured {
	u.Object["spec"].(map[string]any)["imageCatalogRef"] = map[string]any{
		"apiGroup": "postgresql.cnpg.io",
		"kind":     "ClusterImageCatalog",
		"name":     cnpg.CatalogName,
		"major":    int64(17),
	}
	return u
}

func foreignCRD() *apiextensionsv1.CustomResourceDefinition {
	crd := ourCRD(cnpg.CRDNames[0])
	crd.Annotations = map[string]string{"meta.helm.sh/release-name": "cnpg", "meta.helm.sh/release-namespace": "cnpg-system"}
	return crd
}

// With someone else's operator, only the databases on the plugin's catalog
// hold the uninstall up: the others belong to whoever runs that operator, and
// the plugin must not ask anyone to delete them.
func TestUninstallForeignOperator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		databases []client.Object
		wantErr   string
	}{
		{"databases of the other install", []client.Object{cluster("fsc", healthyPhase)}, ""},
		{"a database on the plugin's catalog", []client.Object{cluster("fsc", healthyPhase), onCatalog(cluster("orders", healthyPhase))}, "1 database(s) still exist (team-a/orders)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plugin := newTestPlugin(t)
			kube := newFakeClient(t, append(tt.databases, foreignCRD())...)
			require.NoError(t, applyUserRoles(t.Context(), kube))
			plugin.kube.Store(&kube)

			err := plugin.Uninstall(t.Context(), sdktesting.NewMockHost())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.False(t, plugin.uninstalling)
				return
			}
			require.NoError(t, err)
			assert.True(t, plugin.uninstalling)
			// The other install's database is untouched.
			fsc := cluster("fsc", "")
			require.NoError(t, kube.Get(t.Context(), client.ObjectKeyFromObject(fsc), fsc))
		})
	}
}

// Once an uninstall went ahead, a reconcile or set-up retry must not put back
// what it removed.
func TestNothingRepairedAfterUninstall(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	kube := newFakeClient(t, storageClass("local-path", true), clustersCRD(), catalogCRD())
	plugin.kube.Store(&kube)
	plugin.uninstallOperator = func(context.Context) error { return nil }
	require.NoError(t, plugin.Uninstall(t.Context(), sdktesting.NewMockHost()))

	// newTestPlugin's install fails the test when called.
	require.NoError(t, plugin.Reconcile(t.Context(), sdktesting.NewMockHost()))
	require.NoError(t, plugin.setUpOnce(t.Context(), sdktesting.NewMockHost(), kube))
	catalog := &unstructured.Unstructured{}
	catalog.SetGroupVersionKind(cnpg.CatalogGVK)
	assert.True(t, apierrors.IsNotFound(kube.Get(t.Context(), client.ObjectKey{Name: cnpg.CatalogName}, catalog)))
	for _, role := range userRoles() {
		assert.True(t, apierrors.IsNotFound(kube.Get(t.Context(), client.ObjectKey{Name: role.Name}, &rbacv1.ClusterRole{})), role.Name)
	}
}

// Uninstall waits for a set-up or reconcile in progress, but not for good.
func TestUninstallWaitsForLock(t *testing.T) {
	t.Parallel()
	plugin := newTestPlugin(t)
	kube := newFakeClient(t, clustersCRD())
	plugin.kube.Store(&kube)
	plugin.lock <- struct{}{}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := plugin.Uninstall(ctx, sdktesting.NewMockHost())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry the uninstall")
	assert.False(t, plugin.uninstalling)
}
