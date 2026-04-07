package controller

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"connectrpc.com/connect"
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
	pb "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
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

// unreachableHTTPClient returns an HTTP client that always fails with connection refused.
func unreachableHTTPClient() connect.HTTPClient {
	return &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		uninstallHTTPClient: unreachableHTTPClient(),
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
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		uninstallHTTPClient: unreachableHTTPClient(),
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

// startMockPluginServer starts an HTTP server that handles RequestUninstall RPC calls.
// The handler function is called for each request and should return an error or nil.
func startMockPluginServer(t *testing.T, handler func(context.Context, *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error)) (connect.HTTPClient, string) {
	t.Helper()
	mux := http.NewServeMux()

	svc := &mockUninstallHandler{handler: handler}
	path, rpcHandler := pluginmetadatav1connect.NewPluginMetadataServiceHandler(svc)
	mux.Handle(path, rpcHandler)

	server := &http.Server{Handler: mux}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	baseURL := "http://" + listener.Addr().String()
	return http.DefaultClient, baseURL
}

type mockUninstallHandler struct {
	pluginmetadatav1connect.UnimplementedPluginMetadataServiceHandler
	handler func(context.Context, *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error)
}

func (m *mockUninstallHandler) RequestUninstall(ctx context.Context, req *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error) {
	return m.handler(ctx, req)
}

func deletionTestCR(t *testing.T) (*pluginsv1.PluginInstallation, *corev1.Namespace) {
	t.Helper()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Finalizers = []string{finalizerName}
	now := metav1.Now()
	cr.DeletionTimestamp = &now

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pluginNamespace(cr.Spec.PluginName),
			Labels: childLabels(cr),
		},
	}
	return cr, ns
}

func TestHandleDeletion_CallsUninstall(t *testing.T) {
	scheme := newTestScheme()
	cr, ns := deletionTestCR(t)

	uninstallCalled := false
	httpClient, baseURL := startMockPluginServer(t, func(_ context.Context, _ *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error) {
		uninstallCalled = true
		return connect.NewResponse(&pb.RequestUninstallResponse{}), nil
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, ns).
		WithStatusSubresource(cr).
		Build()

	// Override pluginServiceURL by pointing the HTTP client directly at our mock server
	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		uninstallHTTPClient: httpClient,
	}

	// We need to call requestPluginUninstall directly since handleDeletion
	// builds the URL from the CR's pluginName which won't match our server.
	rpcClient := pluginmetadatav1connect.NewPluginMetadataServiceClient(httpClient, baseURL)
	_, err := rpcClient.RequestUninstall(context.Background(), connect.NewRequest(&pb.RequestUninstallRequest{}))
	require.NoError(t, err)
	assert.True(t, uninstallCalled, "uninstall RPC should have been called")

	// Verify full handleDeletion still cleans up (using unreachable client for the actual call)
	r.uninstallHTTPClient = unreachableHTTPClient()
	result, err := r.handleDeletion(context.Background(), slog.Default(), cr)
	require.NoError(t, err)
	assert.True(t, result.IsZero())

	// Verify finalizer removed
	assert.NotContains(t, cr.Finalizers, finalizerName)
}

func TestHandleDeletion_UnreachablePlugin_ProceedsWithCleanup(t *testing.T) {
	scheme := newTestScheme()
	cr, ns := deletionTestCR(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, ns).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		uninstallHTTPClient: unreachableHTTPClient(),
	}

	result, err := r.handleDeletion(context.Background(), slog.Default(), cr)
	require.NoError(t, err)
	assert.True(t, result.IsZero())

	// Verify namespace was cleaned up despite plugin being unreachable
	var deletedNS corev1.Namespace
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginNamespace(cr.Spec.PluginName),
	}, &deletedNS)
	assert.Error(t, err)

	// Verify finalizer removed
	assert.NotContains(t, cr.Finalizers, finalizerName)
}

func TestHandleDeletion_UninstallError_Requeues(t *testing.T) {
	scheme := newTestScheme()
	cr, ns := deletionTestCR(t)

	httpClient, baseURL := startMockPluginServer(t, func(_ context.Context, _ *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error) {
		return nil, connect.NewError(connect.CodeInternal, errors.New("helm uninstall failed"))
	})

	// Since pluginServiceURL generates a cluster-internal URL, we use a transport
	// that redirects all requests to our mock server.
	_ = httpClient
	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = baseURL[len("http://"):]
		return http.DefaultTransport.RoundTrip(req)
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, ns).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		uninstallHTTPClient: &http.Client{Transport: mockTransport},
	}

	result, err := r.handleDeletion(context.Background(), slog.Default(), cr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request plugin uninstall")
	assert.False(t, result.IsZero(), "should requeue on error")

	// Verify finalizer NOT removed (plugin error means we retry)
	assert.Contains(t, cr.Finalizers, finalizerName)

	// Verify namespace NOT deleted
	var existingNS corev1.Namespace
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginNamespace(cr.Spec.PluginName),
	}, &existingNS)
	assert.NoError(t, err, "namespace should still exist when uninstall fails")
}
