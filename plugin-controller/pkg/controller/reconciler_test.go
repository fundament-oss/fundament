package controller

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/config"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = pluginsv1.AddToScheme(scheme)
	return scheme
}

func testCR() *pluginsv1.PluginInstallation {
	return &pluginsv1.PluginInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-cert-manager",
			Namespace:  "fundament",
			Generation: 1,
		},
		Spec: pluginsv1.PluginInstallationSpec{
			Image:      "ghcr.io/fundament-oss/fundament/cert-manager-plugin:latest",
			PluginName: "cert-manager",
			Version:    "1.0.0",
			Config: map[string]string{
				"LOG_LEVEL": "debug",
			},
		},
	}
}

func TestMutateServiceAccount(t *testing.T) {
	cr := testCR()
	sa := &corev1.ServiceAccount{}
	mutateServiceAccount(sa, cr)

	assert.Equal(t, managedByValue, sa.Labels[labelManagedBy])
	assert.Equal(t, "cert-manager", sa.Labels[labelPlugin])
}

func TestMutateNamespace(t *testing.T) {
	cr := testCR()
	ns := &corev1.Namespace{}
	mutateNamespace(ns, cr)

	assert.Equal(t, managedByValue, ns.Labels[labelManagedBy])
	assert.Equal(t, "cert-manager", ns.Labels[labelPlugin])
}

func TestMutateRoleBinding(t *testing.T) {
	cr := testCR()
	rb := &rbacv1.RoleBinding{}
	mutateRoleBinding(rb, cr)

	assert.Equal(t, managedByValue, rb.Labels[labelManagedBy])
	assert.Equal(t, "admin", rb.RoleRef.Name)
	assert.Equal(t, "ClusterRole", rb.RoleRef.Kind)
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "plugin-cert-manager", rb.Subjects[0].Name)
	assert.Equal(t, "plugin-cert-manager", rb.Subjects[0].Namespace)
}

func TestMutateClusterRoleBinding(t *testing.T) {
	cr := testCR()
	crb := &rbacv1.ClusterRoleBinding{}
	mutateClusterRoleBinding(crb, cr, "cluster-admin")

	assert.Equal(t, managedByValue, crb.Labels[labelManagedBy])
	assert.Equal(t, "cluster-admin", crb.RoleRef.Name)
	assert.Equal(t, "ClusterRole", crb.RoleRef.Kind)
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "plugin-cert-manager", crb.Subjects[0].Name)
	assert.Equal(t, "plugin-cert-manager", crb.Subjects[0].Namespace)
}

func TestMutateDeployment(t *testing.T) {
	cr := testCR()
	envVars := []corev1.EnvVar{
		{Name: "FUNDAMENT_CLUSTER_ID", Value: "test-cluster"},
	}
	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, envVars)

	container := deploy.Spec.Template.Spec.Containers[0]
	assert.Equal(t, cr.Spec.Image, container.Image)
	assert.Equal(t, "cert-manager", container.Name)

	// Should have fundament env vars + config env vars
	foundClusterID := false
	foundLogLevel := false
	for _, env := range container.Env {
		if env.Name == "FUNDAMENT_CLUSTER_ID" {
			foundClusterID = true
			assert.Equal(t, "test-cluster", env.Value)
		}
		if env.Name == "FUNP_LOG_LEVEL" {
			foundLogLevel = true
			assert.Equal(t, "debug", env.Value)
		}
	}
	assert.True(t, foundClusterID, "FUNDAMENT_CLUSTER_ID env var should be present")
	assert.True(t, foundLogLevel, "LOG_LEVEL env var should be present")

	// Health probes
	assert.NotNil(t, container.LivenessProbe)
	assert.NotNil(t, container.ReadinessProbe)
}

func TestMutateService(t *testing.T) {
	cr := testCR()
	svc := &corev1.Service{}
	mutateService(svc, cr)

	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(8080), svc.Spec.Ports[0].Port)
}

func TestReconcileChildren_CreatesResources(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client: fakeClient,
		logger: slog.Default(),
		cfg: config.Config{
			FundamentClusterID: "test-cluster",
			FundamentInstallID: "test-install",
			FundamentOrgID:     "test-org",
		},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	nsName := pluginNamespace(cr.Spec.PluginName)

	// Verify Namespace created
	var ns corev1.Namespace
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: nsName,
	}, &ns)
	require.NoError(t, err)
	assert.Equal(t, managedByValue, ns.Labels[labelManagedBy])

	// Verify ServiceAccount created in plugin namespace
	var sa corev1.ServiceAccount
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: nsName,
	}, &sa)
	require.NoError(t, err)
	assert.Equal(t, managedByValue, sa.Labels[labelManagedBy])

	// Verify RoleBinding created in plugin namespace
	var rb rbacv1.RoleBinding
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: nsName,
	}, &rb)
	require.NoError(t, err)
	assert.Equal(t, "admin", rb.RoleRef.Name)

	// Verify no ClusterRoleBindings created (no clusterRoles requested)
	var crb rbacv1.ClusterRoleBinding
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager-cluster-admin",
	}, &crb)
	assert.Error(t, err, "ClusterRoleBinding should not exist without clusterRoles")

	// Verify Deployment created in plugin namespace
	var deploy appsv1.Deployment
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: nsName,
	}, &deploy)
	require.NoError(t, err)

	// Verify Service created in plugin namespace
	var svc corev1.Service
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: nsName,
	}, &svc)
	require.NoError(t, err)
}

func TestReconcileChildren_CreatesClusterRoleBindings(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.ClusterRoles = []string{"cluster-admin"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client: fakeClient,
		logger: slog.Default(),
		cfg: config.Config{
			FundamentClusterID: "test-cluster",
			FundamentInstallID: "test-install",
			FundamentOrgID:     "test-org",
		},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	// Verify ClusterRoleBinding created
	var crb rbacv1.ClusterRoleBinding
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager-cluster-admin",
	}, &crb)
	require.NoError(t, err)
	assert.Equal(t, "cluster-admin", crb.RoleRef.Name)
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "plugin-cert-manager", crb.Subjects[0].Name)
}

func TestMapPhase(t *testing.T) {
	phase, err := mapPhase("running")
	assert.NoError(t, err)
	assert.Equal(t, pluginsv1.PluginPhaseRunning, phase)

	phase, err = mapPhase("installing")
	assert.NoError(t, err)
	assert.Equal(t, pluginsv1.PluginPhaseDeploying, phase)

	phase, err = mapPhase("degraded")
	assert.NoError(t, err)
	assert.Equal(t, pluginsv1.PluginPhaseDegraded, phase)

	phase, err = mapPhase("failed")
	assert.NoError(t, err)
	assert.Equal(t, pluginsv1.PluginPhaseFailed, phase)

	phase, err = mapPhase("uninstalling")
	assert.NoError(t, err)
	assert.Equal(t, pluginsv1.PluginPhaseTerminating, phase)
}

func TestMapPhase_UnknownReturnsError(t *testing.T) {
	_, err := mapPhase("unknown")
	assert.Error(t, err)

	_, err = mapPhase("")
	assert.Error(t, err)
}

func TestHandleDeletion_CleansUpNamespace(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Finalizers = []string{finalizerName}
	now := metav1.Now()
	cr.DeletionTimestamp = &now

	// Pre-create the plugin namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pluginNamespace(cr.Spec.PluginName),
			Labels: childLabels(cr),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, ns).
		Build()

	r := &Reconciler{
		client: fakeClient,
		logger: slog.Default(),
	}

	_, err := r.handleDeletion(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	// Verify Namespace deleted
	var deletedNS corev1.Namespace
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginNamespace(cr.Spec.PluginName),
	}, &deletedNS)
	assert.Error(t, err)

	// Verify finalizer removed
	assert.NotContains(t, cr.Finalizers, finalizerName)
}

func TestHandleDeletion_CleansUpClusterRoleBindings(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.ClusterRoles = []string{"cluster-admin"}
	cr.Finalizers = []string{finalizerName}
	now := metav1.Now()
	cr.DeletionTimestamp = &now

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pluginNamespace(cr.Spec.PluginName),
			Labels: childLabels(cr),
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterRoleBindingName(cr.Spec.PluginName, "cluster-admin"),
			Labels: childLabels(cr),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, ns, crb).
		Build()

	r := &Reconciler{
		client: fakeClient,
		logger: slog.Default(),
	}

	_, err := r.handleDeletion(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	// Verify ClusterRoleBinding deleted
	var deletedCRB rbacv1.ClusterRoleBinding
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: clusterRoleBindingName(cr.Spec.PluginName, "cluster-admin"),
	}, &deletedCRB)
	assert.Error(t, err)

	// Verify Namespace deleted
	var deletedNS corev1.Namespace
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginNamespace(cr.Spec.PluginName),
	}, &deletedNS)
	assert.Error(t, err)

	// Verify finalizer removed
	assert.NotContains(t, cr.Finalizers, finalizerName)
}

func TestPluginNamespace(t *testing.T) {
	assert.Equal(t, "plugin-cert-manager", pluginNamespace("cert-manager"))
	assert.Equal(t, "plugin-cert-manager-test", pluginNamespace("cert-manager-test"))
}

func TestPluginServiceURL(t *testing.T) {
	url := pluginServiceURL("cert-manager")
	assert.Equal(t, "http://plugin-cert-manager.plugin-cert-manager.svc.cluster.local:8080", url)
}
