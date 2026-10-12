package main

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	sdktesting "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/testing"
	"github.com/fundament-oss/fundament/plugins/internal/cnpg"
)

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	plugin, err := newPlugin()
	require.NoError(t, err)

	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin

	// No Installer: uninstalling removes only the UI. The operator is shared
	// with openfsc, and the databases must outlive the plugin.
	_, isInstaller := any(plugin).(pluginruntime.Installer)
	assert.False(t, isInstaller)
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

var clustersCRD = &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "clusters.postgresql.cnpg.io"}}

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
			kube := newFakeClient(t, append(tt.objs, clustersCRD, operatorDeployment())...)
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
	kube := newFakeClient(t, storageClass("local-path", true), clustersCRD)
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
		// openfsc's install of the same release is still running.
		{"in progress", fmt.Errorf("%w: release is pending-upgrade", cnpg.ErrInProgress), pluginruntime.PhaseInstalling},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plugin := newTestPlugin(t)
			kube := newFakeClient(t, clustersCRD)
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
