package main

import (
	"testing"

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
			plugin, err := newPlugin()
			require.NoError(t, err)
			kube := newFakeClient(t, append(tt.objs, clustersCRD)...)
			plugin.kube.Store(&kube)
			host := sdktesting.NewMockHost()

			require.NoError(t, plugin.Reconcile(t.Context(), host))
			assert.Equal(t, pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: tt.message}, host.CurrentStatus())
		})
	}
}

func TestReconcileDegradesWithoutCRD(t *testing.T) {
	t.Parallel()
	plugin, err := newPlugin()
	require.NoError(t, err)
	kube := newFakeClient(t)
	plugin.kube.Store(&kube)
	host := sdktesting.NewMockHost()

	require.Error(t, plugin.Reconcile(t.Context(), host))
	assert.Equal(t, pluginruntime.PhaseDegraded, host.CurrentStatus().Phase)
}

// Reconcile runs on a ticker from the start; before Start has a client it is a
// no-op rather than a nil dereference.
func TestReconcileBeforeStart(t *testing.T) {
	t.Parallel()
	plugin, err := newPlugin()
	require.NoError(t, err)
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
		"fundament-cloudnativepg-admin": {"get", "list", "watch", "create"},
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

// Project admins must not be able to update or delete a Cluster, including
// through the console's generated views and kubectl.
func TestUserRolesGrantNoModification(t *testing.T) {
	t.Parallel()
	for _, role := range userRoles() {
		for _, rule := range role.Rules {
			for _, verb := range []string{"update", "patch", "delete", "deletecollection", "*"} {
				assert.NotContains(t, rule.Verbs, verb, "%s must not grant %s", role.Name, verb)
			}
		}
	}
}
