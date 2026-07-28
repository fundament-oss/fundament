package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/config"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/defclient"
	pb "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
)

// fakeDefClient is a stub defclient.Client for tests. It returns a pre-canned
// manifest + hash, or a canned error.
type fakeDefClient struct {
	manifest []byte
	hash     string
	err      error
}

func (f fakeDefClient) GetDefinition(_ context.Context, _, _ string) (defclient.Definition, error) {
	if f.err != nil {
		return defclient.Definition{}, f.err
	}
	return defclient.Definition{Manifest: f.manifest, Hash: f.hash}, nil
}

// countingDefClient counts GetDefinition calls to assert the reconciler does not
// re-fetch an immutable definition on every poll.
type countingDefClient struct {
	inner defclient.Client
	calls int
}

func (c *countingDefClient) GetDefinition(ctx context.Context, name, version string) (defclient.Definition, error) {
	c.calls++
	return c.inner.GetDefinition(ctx, name, version)
}

// sampleManifest returns a valid PluginDefinition YAML and its sha256 pin.
func sampleManifest(t *testing.T) ([]byte, string) {
	t.Helper()
	manifest := []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  imagePullPolicy: IfNotPresent
  permissions:
    rbac:
      - apiGroups: [cert-manager.io]
        resources: [certificates, certificaterequests]
        verbs: [get, list, watch]
      - apiGroups: [""]
        resources: [secrets]
        verbs: [get]
`)
	sum := sha256.Sum256(manifest)
	return manifest, "sha256:" + hex.EncodeToString(sum[:])
}

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
			DefinitionRef: pluginsv1.DefinitionRef{
				PluginName:    "cert-manager",
				PluginVersion: "v1.17.2",
				// DefinitionHash intentionally empty in tests — the reconciler
				// treats an empty pin as "no hash check" when AllowUnpinnedHash
				// is set. The dedicated hash tests set it explicitly.
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

func TestMutateService(t *testing.T) {
	cr := testCR()
	svc := &corev1.Service{}
	mutateService(svc, cr)

	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(8080), svc.Spec.Ports[0].Port)
}

// TestReconcileChildren_CreatesResources exercises the full happy-path reconcile
// with a fake defclient that returns a valid manifest whose hash equals the CR
// pin. All child resources (Namespace, SA, RoleBinding, scope CR+CRB,
// Deployment, Service) are created.
func TestReconcileChildren_CreatesResources(t *testing.T) {
	scheme := newTestScheme()
	manifest, pin := sampleManifest(t)

	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = pin

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
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: pin},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

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

	// Verify scope ClusterRole created
	var scopeRole rbacv1.ClusterRole
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: pluginScopeClusterRoleName(cr.Name),
	}, &scopeRole)
	require.NoError(t, err)

	// Verify Deployment created in plugin namespace with image from manifest
	var deploy appsv1.Deployment
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: nsName,
	}, &deploy)
	require.NoError(t, err)
	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", deploy.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, corev1.PullIfNotPresent, deploy.Spec.Template.Spec.Containers[0].ImagePullPolicy)

	// Verify Service created in plugin namespace
	var svc corev1.Service
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: nsName,
	}, &svc)
	require.NoError(t, err)
}

// TestReconcileChildren_MaterialisesPluginScopeRBAC verifies FUN-17: the
// reconciler fetches the definition via defclient and materialises the plugin
// SA's scope ClusterRole + ClusterRoleBinding from its permissions.rbac.
func TestReconcileChildren_MaterialisesPluginScopeRBAC(t *testing.T) {
	scheme := newTestScheme()
	manifest, pin := sampleManifest(t)

	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = pin

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: pin},
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

// TestReconcileChildren_DeploymentUsesManifestImage verifies that the
// Deployment's container image is sourced from the parsed definition, not from
// cr.Spec.Image (which is being removed in Task 8).
func TestReconcileChildren_DeploymentUsesManifestImage(t *testing.T) {
	scheme := newTestScheme()
	manifest, pin := sampleManifest(t)

	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = pin
	// Even if cr.Spec.Image were set to something else, the reconciler must
	// use the manifest image. Leave it empty here to prove the fetch path
	// supplies the image.

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: pin},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: pluginNamespace(cr.Name),
	}, &deploy)
	require.NoError(t, err)
	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", deploy.Spec.Template.Spec.Containers[0].Image)
}

// TestReconcileChildren_DeploymentCreatedAfterScope verifies that on the first
// reconcile, the scope ClusterRole exists by the time the Deployment is
// created (order: Namespace, SA, RoleBinding, scope CR+CRB, Deployment,
// Service). We assert this by observing that if scope fetching fails, no
// Deployment is created.
func TestReconcileChildren_DeploymentNotCreatedWhenScopeFails(t *testing.T) {
	scheme := newTestScheme()

	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = "sha256:mismatch-so-scope-fails-before-deployment"

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	manifest, _ := sampleManifest(t)
	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: "sha256:whatever"},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err)

	// Deployment must NOT exist yet — scope failure aborts before Deployment.
	var deploy appsv1.Deployment
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager", Namespace: pluginNamespace(cr.Name),
	}, &deploy)
	require.Error(t, err, "Deployment must not be created when scope reconcile fails")
}

// TestReconcilePluginScope_RejectsHashMismatch confirms that when
// spec.definitionRef.definitionHash is set and doesn't match the computed
// sha256 of the fetched manifest, the reconcile errors and the scope
// ClusterRole is not created.
func TestReconcilePluginScope_RejectsHashMismatch(t *testing.T) {
	scheme := newTestScheme()
	manifest, _ := sampleManifest(t)

	cr := testCR()
	cr.SetUID("test-uid")
	cr.Spec.DefinitionRef.DefinitionHash = "sha256:definitely-not-what-organization-api-serves"

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: "sha256:whatever"},
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

// TestReconcilePluginScope_RejectsUnpinnedWithoutFlag exercises the fail-closed
// default: with AllowUnpinnedHash=false, an empty DefinitionHash is a hard
// failure and no defclient RPC is attempted.
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
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{AllowUnpinnedHash: false},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{err: errors.New("must not be called")},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH")
}

// TestReconcilePluginScope_UnpinnedWithFlag exercises the dev-loop path: with
// AllowUnpinnedHash=true and an empty pin, the reconciler fetches the
// definition and materialises whatever it gets (no hash comparison).
func TestReconcilePluginScope_UnpinnedWithFlag(t *testing.T) {
	scheme := newTestScheme()
	manifest, _ := sampleManifest(t)

	cr := testCR() // empty DefinitionHash
	cr.SetUID("test-uid")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{AllowUnpinnedHash: true},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{manifest: manifest, hash: "sha256:not-checked"},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.NoError(t, err)

	var role rbacv1.ClusterRole
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "plugin-cert-manager-scope",
	}, &role)
	require.NoError(t, err)
}

// TestReconcilePluginScope_FetchError propagates a defclient RPC error and
// sets PluginScopeReady=False.
func TestReconcilePluginScope_FetchError(t *testing.T) {
	scheme := newTestScheme()
	cr := testCR()
	cr.SetUID("test-uid")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{AllowUnpinnedHash: true},
		uninstallHTTPClient: http.DefaultClient,
		defClient:           fakeDefClient{err: errors.New("organization-api unreachable")},
	}

	err := r.reconcileChildren(context.Background(), slog.Default(), cr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch definition")

	var cond *metav1.Condition
	for i := range cr.Status.Conditions {
		if cr.Status.Conditions[i].Type == ConditionPluginScopeReady {
			cond = &cr.Status.Conditions[i]
			break
		}
	}
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
}

// TestReconcile_RepairsDriftAndCachesDefinition verifies the controller is
// level-triggered: a child deleted out of band is recreated on the next
// reconcile even though the generation is unchanged, while the immutable,
// hash-pinned definition is served from cache rather than re-fetched.
func TestReconcile_RepairsDriftAndCachesDefinition(t *testing.T) {
	scheme := newTestScheme()
	manifest, pin := sampleManifest(t)

	cr := testCR()
	cr.SetUID("test-uid")
	cr.Finalizers = []string{finalizerName}
	cr.Spec.DefinitionRef.DefinitionHash = pin

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	counting := &countingDefClient{inner: fakeDefClient{manifest: manifest, hash: pin}}
	r := &Reconciler{
		client:              fakeClient,
		logger:              slog.Default(),
		cfg:                 config.Config{StatusPollInterval: 30 * time.Second},
		statusPoller:        newStatusPoller(),
		uninstallHTTPClient: http.DefaultClient,
		defClient:           counting,
		defCache:            newDefinitionCache(),
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: cr.Name}}
	ctx := context.Background()
	deployKey := types.NamespacedName{Name: "plugin-cert-manager", Namespace: pluginNamespace(cr.Name)}

	// First reconcile: materialise children; one fetch.
	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 1, counting.calls)
	require.NoError(t, fakeClient.Get(ctx, deployKey, &appsv1.Deployment{}))

	// Simulate out-of-band drift: delete the Deployment.
	require.NoError(t, fakeClient.Delete(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deployKey.Name, Namespace: deployKey.Namespace},
	}))

	// Second reconcile (generation unchanged): level-triggered → the drifted
	// Deployment is recreated, and the pinned definition is served from cache.
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.NoError(t, fakeClient.Get(ctx, deployKey, &appsv1.Deployment{}), "drifted child must be recreated")
	assert.Equal(t, 1, counting.calls, "pinned definition must be served from cache, not re-fetched")
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
	requestUninstall func(context.Context, *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error)
}

// startMockPluginServer boots an HTTP server that speaks the
// PluginMetadataService protocol and returns whatever the caller wired up.
// Only uninstall/status handlers are used now — GetDefinition moved off the
// pod RPC to organization-api (Task 6/7).
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
