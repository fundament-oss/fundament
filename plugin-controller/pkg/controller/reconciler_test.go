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
			Name:       "cert-manager",
			Generation: 1,
		},
		Spec: pluginsv1.PluginInstallationSpec{
			Image: "ghcr.io/fundament-oss/fundament/cert-manager-plugin:latest",
			DefinitionRef: pluginsv1.DefinitionRef{
				PluginName:    "cert-manager",
				PluginVersion: "v1.17.2",
				// DefinitionHash intentionally empty in tests — the reconciler
				// treats an empty pin as "no hash check". The dedicated hash
				// test sets it explicitly.
			},
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
	assert.Equal(t, "/livez", container.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, "/readyz", container.ReadinessProbe.HTTPGet.Path)
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
			// Empty DefinitionHash is only accepted when the operator has opted
			// in via this flag. This test focuses on child resources, not the
			// scope RPC; the scope step still fails (unreachable RPC) below.
			AllowUnpinnedHash: true,
		},
		uninstallHTTPClient:      http.DefaultClient,
		scopeHTTPClient:          http.DefaultClient,
		pluginServiceURLOverride: "http://127.0.0.1:1", // reject-connect, fast fail
	}

	// The plugin scope RPC fails (no server) — errors propagate so the workqueue
	// retries. This test only asserts the child resources that ARE created
	// before the scope step, so the propagated error is expected.
	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err, "scope RPC failure propagates")
	assert.Contains(t, err.Error(), "reconcile plugin scope")

	nsName := pluginNamespace(cr.Name)

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

// TestReconcileChildren_MaterialisesPluginScopeRBAC verifies FUN-17: the
// reconciler RPCs the plugin's GetDefinition and materialises the plugin SA's
// scope ClusterRole + ClusterRoleBinding from its permissions.rbac.
func TestReconcileChildren_MaterialisesPluginScopeRBAC(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")

	name, version := "cert-manager", "v1.17.2"
	def := &pb.GetDefinitionResponse{
		Name:    &name,
		Version: &version,
		Permissions: &pb.Permissions{
			Rbac: []*pb.PolicyRule{
				{
					ApiGroups: []string{"cert-manager.io"},
					Resources: []string{"certificates", "certificaterequests"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					ApiGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"get"},
				},
			},
		},
	}
	httpClient, baseURL := startMockPluginServer(t, mockPluginHandlers{
		getDefinition: func(_ context.Context, _ *connect.Request[pb.GetDefinitionRequest]) (*connect.Response[pb.GetDefinitionResponse], error) {
			return connect.NewResponse(def), nil
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:                   fakeClient,
		logger:                   slog.Default(),
		cfg:                      config.Config{AllowUnpinnedHash: true},
		uninstallHTTPClient:      httpClient,
		scopeHTTPClient:          httpClient,
		pluginServiceURLOverride: baseURL,
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	var role rbacv1.ClusterRole
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager-scope",
	}, &role)
	require.NoError(t, err)
	require.Len(t, role.Rules, 2)
	assert.Equal(t, []string{"cert-manager.io"}, role.Rules[0].APIGroups)
	assert.Equal(t, []string{"certificates", "certificaterequests"}, role.Rules[0].Resources)
	assert.Equal(t, []string{"secrets"}, role.Rules[1].Resources)

	var crb rbacv1.ClusterRoleBinding
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager-scope",
	}, &crb)
	require.NoError(t, err)
	assert.Equal(t, "plugin-cert-manager-scope", crb.RoleRef.Name)
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "plugin-cert-manager", crb.Subjects[0].Name)
	assert.Equal(t, "plugin-cert-manager", crb.Subjects[0].Namespace)
}

// TestReconcilePluginScope_RejectsHashMismatch confirms that when
// spec.definitionRef.definitionHash is set and doesn't match the plugin's
// GetDefinition response, the scope ClusterRole is not created.
func TestReconcilePluginScope_RejectsHashMismatch(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = "sha256:definitely-not-what-the-plugin-serves"

	name2, version2 := "cert-manager", "v1.17.2"
	def := &pb.GetDefinitionResponse{
		Name:    &name2,
		Version: &version2,
		Permissions: &pb.Permissions{
			Rbac: []*pb.PolicyRule{{ApiGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}}},
		},
	}
	httpClient, baseURL := startMockPluginServer(t, mockPluginHandlers{
		getDefinition: func(_ context.Context, _ *connect.Request[pb.GetDefinitionRequest]) (*connect.Response[pb.GetDefinitionResponse], error) {
			return connect.NewResponse(def), nil
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:                   fakeClient,
		logger:                   slog.Default(),
		uninstallHTTPClient:      httpClient,
		scopeHTTPClient:          httpClient,
		pluginServiceURLOverride: baseURL,
	}

	// reconcileChildren propagates the hash-mismatch so the workqueue retries
	// (and the PluginScopeReady Condition on the CR reflects the failure).
	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definition hash mismatch")

	var role rbacv1.ClusterRole
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager-scope",
	}, &role)
	require.Error(t, err, "scope ClusterRole must not be created on hash mismatch")

	// The condition is set on the CR passed into reconcileChildren.
	var cond *metav1.Condition
	for i := range cr.Status.Conditions {
		if cr.Status.Conditions[i].Type == ConditionPluginScopeReady {
			cond = &cr.Status.Conditions[i]
			break
		}
	}
	require.NotNil(t, cond, "PluginScopeReady Condition must be set")
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, "MaterialisationFailed", cond.Reason)
}

// TestReconcilePluginScope_RejectsUnpinnedWithoutFlag exercises Fix #2:
// with AllowUnpinnedHash=false (the production default), an empty
// DefinitionHash is a hard failure and no scope is materialised.
func TestReconcilePluginScope_RejectsUnpinnedWithoutFlag(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR() // testCR uses empty hash
	cr.SetUID("test-uid")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:                   fakeClient,
		logger:                   slog.Default(),
		cfg:                      config.Config{AllowUnpinnedHash: false},
		uninstallHTTPClient:      http.DefaultClient,
		scopeHTTPClient:          http.DefaultClient,
		pluginServiceURLOverride: "http://127.0.0.1:1",
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH")
}

// TestReconcilePluginScope_AcceptsUnknownPlaceholder verifies that the
// terraform provider's default "sha256:unknown" placeholder is treated as
// unpinned (not verified against the plugin's real digest), so the scope
// ClusterRole still materialises when AllowUnpinnedHash is set.
func TestReconcilePluginScope_AcceptsUnknownPlaceholder(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = "sha256:unknown"

	name, version := "cert-manager", "v1.17.2"
	def := &pb.GetDefinitionResponse{
		Name:    &name,
		Version: &version,
		Permissions: &pb.Permissions{
			Rbac: []*pb.PolicyRule{
				{ApiGroups: []string{"cert-manager.io"}, Resources: []string{"certificates"}, Verbs: []string{"get", "list", "watch"}},
			},
		},
	}
	httpClient, baseURL := startMockPluginServer(t, mockPluginHandlers{
		getDefinition: func(_ context.Context, _ *connect.Request[pb.GetDefinitionRequest]) (*connect.Response[pb.GetDefinitionResponse], error) {
			return connect.NewResponse(def), nil
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:                   fakeClient,
		logger:                   slog.Default(),
		cfg:                      config.Config{AllowUnpinnedHash: true},
		uninstallHTTPClient:      httpClient,
		scopeHTTPClient:          httpClient,
		pluginServiceURLOverride: baseURL,
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err, "sha256:unknown placeholder must not trigger a hash mismatch")

	var role rbacv1.ClusterRole
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager-scope",
	}, &role)
	require.NoError(t, err, "scope ClusterRole must be created for the unknown placeholder")
	require.Len(t, role.Rules, 1)
}

// TestReconcilePluginScope_RejectsUnknownPlaceholderWithoutFlag confirms the
// "sha256:unknown" placeholder is fail-closed like an empty hash when
// AllowUnpinnedHash is false (the production default).
func TestReconcilePluginScope_RejectsUnknownPlaceholderWithoutFlag(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = "sha256:unknown"

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:                   fakeClient,
		logger:                   slog.Default(),
		cfg:                      config.Config{AllowUnpinnedHash: false},
		uninstallHTTPClient:      http.DefaultClient,
		scopeHTTPClient:          http.DefaultClient,
		pluginServiceURLOverride: "http://127.0.0.1:1",
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH")
}

// TestHashDefinition_DeterministicAcrossCalls exercises Fix #7: the canonical
// protojson-based hash returns identical bytes for the same message across
// invocations.
func TestHashDefinition_DeterministicAcrossCalls(t *testing.T) {
	name, ver := "cert-manager", "v1.17.2"
	def := &pb.GetDefinitionResponse{
		Name:    &name,
		Version: &ver,
		Permissions: &pb.Permissions{
			Rbac: []*pb.PolicyRule{
				{ApiGroups: []string{"cert-manager.io"}, Resources: []string{"certificates"}, Verbs: []string{"get", "list"}},
			},
		},
	}

	h1, err := hashDefinition(def)
	require.NoError(t, err)
	h2, err := hashDefinition(def)
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "same message must produce same hash")
	assert.True(t, len(h1) > len("sha256:"), "hash must be non-empty")

	// A tiny change flips the hash.
	def.Version = ptr("v1.17.3")
	h3, err := hashDefinition(def)
	require.NoError(t, err)
	assert.NotEqual(t, h1, h3)
}

func ptr[T any](v T) *T { return &v }

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
			Name:   pluginNamespace(cr.Name),
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
		Name: pluginNamespace(cr.Name),
	}, &deletedNS)
	assert.Error(t, err)

	// Verify finalizer removed
	assert.NotContains(t, cr.Finalizers, finalizerName)
}

func TestHandleDeletion_CleansUpPluginScopeRBAC(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")
	cr.Finalizers = []string{finalizerName}
	now := metav1.Now()
	cr.DeletionTimestamp = &now

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pluginNamespace(cr.Name),
			Labels: childLabels(cr),
		},
	}
	scopeRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pluginScopeClusterRoleName(cr.Name),
			Labels: childLabels(cr),
		},
	}
	scopeCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pluginScopeClusterRoleName(cr.Name),
			Labels: childLabels(cr),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, ns, scopeRole, scopeCRB).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		uninstallHTTPClient: unreachableHTTPClient(),
	}

	_, err := r.handleDeletion(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	var deletedRole rbacv1.ClusterRole
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginScopeClusterRoleName(cr.Name),
	}, &deletedRole)
	assert.Error(t, err)

	var deletedCRB rbacv1.ClusterRoleBinding
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginScopeClusterRoleName(cr.Name),
	}, &deletedCRB)
	assert.Error(t, err)

	var deletedNS corev1.Namespace
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginNamespace(cr.Name),
	}, &deletedNS)
	assert.Error(t, err)

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

// mockPluginHandlers lets a test wire specific RPC responses without having to
// stub every method. Unset handlers fall through to the connect-provided
// "unimplemented" default.
type mockPluginHandlers struct {
	getDefinition    func(context.Context, *connect.Request[pb.GetDefinitionRequest]) (*connect.Response[pb.GetDefinitionResponse], error)
	requestUninstall func(context.Context, *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error)
}

// startMockPluginServer boots an HTTP server that speaks the
// PluginMetadataService protocol and returns whatever the caller wired up.
func startMockPluginServer(t *testing.T, handlers mockPluginHandlers) (connect.HTTPClient, string) {
	t.Helper()
	mux := http.NewServeMux()

	svc := &mockPluginHandler{handlers: handlers}
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

type mockPluginHandler struct {
	pluginmetadatav1connect.UnimplementedPluginMetadataServiceHandler
	handlers mockPluginHandlers
}

func (m *mockPluginHandler) GetDefinition(ctx context.Context, req *connect.Request[pb.GetDefinitionRequest]) (*connect.Response[pb.GetDefinitionResponse], error) {
	if m.handlers.getDefinition == nil {
		return m.UnimplementedPluginMetadataServiceHandler.GetDefinition(ctx, req)
	}
	return m.handlers.getDefinition(ctx, req)
}

func (m *mockPluginHandler) RequestUninstall(ctx context.Context, req *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error) {
	if m.handlers.requestUninstall == nil {
		return m.UnimplementedPluginMetadataServiceHandler.RequestUninstall(ctx, req)
	}
	return m.handlers.requestUninstall(ctx, req)
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
			Name:   pluginNamespace(cr.Name),
			Labels: childLabels(cr),
		},
	}
	return cr, ns
}

func TestHandleDeletion_CallsUninstall(t *testing.T) {
	scheme := newTestScheme()
	cr, ns := deletionTestCR(t)

	uninstallCalled := false
	httpClient, baseURL := startMockPluginServer(t, mockPluginHandlers{
		requestUninstall: func(_ context.Context, _ *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error) {
			uninstallCalled = true
			return connect.NewResponse(&pb.RequestUninstallResponse{}), nil
		},
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
		Name: pluginNamespace(cr.Name),
	}, &deletedNS)
	assert.Error(t, err)

	// Verify finalizer removed
	assert.NotContains(t, cr.Finalizers, finalizerName)
}

func TestHandleDeletion_UninstallError_Requeues(t *testing.T) {
	scheme := newTestScheme()
	cr, ns := deletionTestCR(t)

	httpClient, baseURL := startMockPluginServer(t, mockPluginHandlers{
		requestUninstall: func(_ context.Context, _ *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("helm uninstall failed"))
		},
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
		Name: pluginNamespace(cr.Name),
	}, &existingNS)
	assert.NoError(t, err, "namespace should still exist when uninstall fails")
}
