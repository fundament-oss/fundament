package main

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
	sdktesting "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/testing"
)

func TestMain(m *testing.M) {
	// The fake client never sets the Established condition.
	waitEstablished = func(context.Context, client.Client, []string) error { return nil }
	os.Exit(m.Run())
}

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	plugin, err := newPlugin()
	require.NoError(t, err)

	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Reconciler = plugin
	// For Uninstall, which removes the release.
	var _ pluginruntime.Installer = plugin
}

// fakeHelm records the helm calls instead of shelling out to helm.
type fakeHelm struct {
	mu         sync.Mutex
	status     *helm.ReleaseStatus
	installErr error
	installs   []string
	uninstalls int
}

func (f *fakeHelm) Status(context.Context, string) (*helm.ReleaseStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

func (f *fakeHelm) InstallFromRepo(_ context.Context, _, _, _, version string, values map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if values["service.type"] != "ClusterIP" {
		return errors.New("the install must keep the Service inside the cluster")
	}
	if values[`deploymentAnnotations.zot\.fundament\.io/values-hash`] != valuesHash() {
		return errors.New("the install must put the values hash on the Deployment")
	}
	f.installs = append(f.installs, version)
	return f.installErr
}

func (f *fakeHelm) Uninstall(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstalls++
	return nil
}

func deployed(version string) *helm.ReleaseStatus {
	return &helm.ReleaseStatus{Status: "deployed", ChartVersion: version}
}

func zotDeployment(available int32) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        release,
			Namespace:   namespace,
			Annotations: map[string]string{valuesHashAnnotation: valuesHash()},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "zot", Image: "ghcr.io/project-zot/zot:v2.1.23"}}}},
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: available},
	}
}

func deploymentWithHash(hash string) *appsv1.Deployment {
	d := zotDeployment(1)
	d.Annotations[valuesHashAnnotation] = hash
	return d
}

var testNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// newTestPlugin returns a plugin on a fake Kubernetes client and a fake helm.
// Objects are built per test: the fake client sets their resourceVersion, and
// the tests run in parallel.
func newTestPlugin(t *testing.T, h *fakeHelm, objs ...client.Object) (*Plugin, client.Client) {
	t.Helper()
	plugin, err := newPlugin()
	require.NoError(t, err)
	// The fake client tracks the Registry kind only once the scheme knows it.
	plugin.scheme.AddKnownTypeWithName(registryGVK, &unstructured.Unstructured{})
	plugin.scheme.AddKnownTypeWithName(registryGVK.GroupVersion().WithKind("RegistryList"), &unstructured.UnstructuredList{})
	kube := fake.NewClientBuilder().WithScheme(plugin.scheme).WithObjects(objs...).WithStatusSubresource(newRegistry()).Build()
	plugin.helm = h
	plugin.now = func() time.Time { return testNow }
	plugin.newClient = func() (client.Client, error) { return kube, nil }
	return plugin, kube
}

func TestInstall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   *helm.ReleaseStatus
		objs     []client.Object
		installs []string
		errIs    error
		errText  string
	}{
		{
			name:     "no release",
			installs: []string{chartVersion},
		},
		{
			name:   "deployed at the pinned version",
			status: deployed(chartVersion),
			objs:   []client.Object{zotDeployment(1)},
		},
		{
			name:     "deployed, deployment deleted",
			status:   deployed(chartVersion),
			installs: []string{chartVersion},
		},
		{
			// A plugin version that changed chartValues but kept chartVersion.
			name:     "deployed with other values",
			status:   deployed(chartVersion),
			objs:     []client.Object{deploymentWithHash("0123456789abcdef")},
			installs: []string{chartVersion},
		},
		{
			// A newer plugin pod upgraded it; a downgrade would restart zot.
			name:   "deployed at a newer version",
			status: deployed("0.1.200"),
			objs:   []client.Object{deploymentWithHash("0123456789abcdef")},
		},
		{
			name:    "deployed at a newer version, deployment deleted",
			status:  deployed("0.1.200"),
			errText: "left for the newer plugin to repair",
		},
		{
			name:     "deployed at an older version",
			status:   deployed("0.1.100"),
			objs:     []client.Object{zotDeployment(1)},
			installs: []string{chartVersion},
		},
		{
			name:     "failed release",
			status:   &helm.ReleaseStatus{Status: "failed", ChartVersion: chartVersion},
			objs:     []client.Object{zotDeployment(1)},
			installs: []string{chartVersion},
		},
		{
			name:   "pending upgrade in progress",
			status: &helm.ReleaseStatus{Status: "pending-upgrade", LastDeployed: testNow.Add(-time.Minute)},
			errIs:  errInProgress,
		},
		{
			name:    "pending install stuck",
			status:  &helm.ReleaseStatus{Status: "pending-install", LastDeployed: testNow.Add(-time.Hour)},
			errText: "helm -n zot uninstall zot",
		},
		{
			name:    "pending upgrade stuck",
			status:  &helm.ReleaseStatus{Status: "pending-upgrade", LastDeployed: testNow.Add(-time.Hour)},
			errText: "helm -n zot rollback zot",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &fakeHelm{status: tt.status}
			plugin, kube := newTestPlugin(t, h, tt.objs...)

			err := plugin.install(t.Context(), kube)
			switch {
			case tt.errIs != nil:
				require.ErrorIs(t, err, tt.errIs)
			case tt.errText != "":
				require.ErrorContains(t, err, tt.errText)
				assert.NotErrorIs(t, err, errInProgress)
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tt.installs, h.installs)
		})
	}
}

func TestInstallReportsConcurrentInstallAsInProgress(t *testing.T) {
	t.Parallel()
	h := &fakeHelm{installErr: errors.New("helm install failed: Error: UPGRADE FAILED: another operation (install/upgrade/rollback) is in progress")}
	plugin, kube := newTestPlugin(t, h)

	err := plugin.install(t.Context(), kube)
	require.ErrorIs(t, err, errInProgress)
	assert.Equal(t, pluginruntime.PhaseInstalling, failurePhase(err))
}

func TestSetUpReportsRunning(t *testing.T) {
	t.Parallel()
	h := &fakeHelm{status: deployed(chartVersion)}
	plugin, kube := newTestPlugin(t, h, zotDeployment(1))
	host := sdktesting.NewMockHost()

	require.NoError(t, plugin.setUpOnce(t.Context(), host, kube))
	assert.True(t, host.IsReady())
	assert.Equal(t, pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "zot is running at zot.zot.svc:5000"}, host.CurrentStatus())

	// The console shows the same status, from the plugin's own CRD.
	require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: consoleCRDName}, &apiextensionsv1.CustomResourceDefinition{}))
	registry := newRegistry()
	require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: release}, registry))
	got, _, _ := unstructured.NestedStringMap(registry.Object, "status")
	assert.Equal(t, map[string]string{
		"phase":     "running",
		"message":   "zot is running at zot.zot.svc:5000",
		"address":   "zot.zot.svc:5000",
		"image":     "ghcr.io/project-zot/zot:v2.1.23",
		"available": "1/1",
	}, got)
	chart, _, _ := unstructured.NestedString(registry.Object, "spec", "chartVersion")
	assert.Equal(t, chartVersion, chart)
}

func TestSetUpReportsFailure(t *testing.T) {
	t.Parallel()
	h := &fakeHelm{installErr: errors.New("helm install failed: timed out waiting for the condition")}
	plugin, kube := newTestPlugin(t, h)
	host := sdktesting.NewMockHost()

	require.Error(t, plugin.setUpOnce(t.Context(), host, kube))
	assert.False(t, host.IsReady())
	assert.Equal(t, pluginruntime.PhaseDegraded, host.CurrentStatus().Phase)
	assert.Contains(t, host.CurrentStatus().Message, "timed out")
}

func TestReconcile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		objs     []client.Object
		installs []string
		status   pluginruntime.PluginStatus
	}{
		{
			name:   "running",
			objs:   []client.Object{zotDeployment(1)},
			status: pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "zot is running at zot.zot.svc:5000"},
		},
		{
			name:   "not ready",
			objs:   []client.Object{zotDeployment(0)},
			status: pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: "zot is not ready: 0 of 1 replicas available"},
		},
		{
			// Helm still says deployed, so nothing else would put it back. The
			// fake helm creates nothing, so the status still shows it missing.
			name:     "deployment deleted",
			installs: []string{chartVersion},
			status:   pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: "deployment zot/zot is missing"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &fakeHelm{status: deployed(chartVersion)}
			plugin, kube := newTestPlugin(t, h, tt.objs...)
			plugin.kube = kube
			host := sdktesting.NewMockHost()

			require.NoError(t, plugin.Reconcile(t.Context(), host))
			assert.Equal(t, tt.installs, h.installs)
			assert.Equal(t, tt.status, host.CurrentStatus())
		})
	}
}

// Before Start has set zot up, Reconcile leaves the install to Start.
func TestReconcileBeforeSetUpDoesNothing(t *testing.T) {
	t.Parallel()
	h := &fakeHelm{}
	plugin, _ := newTestPlugin(t, h)

	require.NoError(t, plugin.Reconcile(t.Context(), sdktesting.NewMockHost()))
	assert.Empty(t, h.installs)
}

func TestUninstall(t *testing.T) {
	t.Parallel()
	h := &fakeHelm{status: deployed(chartVersion)}
	plugin, kube := newTestPlugin(t, h, zotDeployment(1))
	plugin.kube = kube
	host := sdktesting.NewMockHost()

	require.NoError(t, plugin.Reconcile(t.Context(), host))
	require.NoError(t, kube.Get(t.Context(), client.ObjectKey{Name: consoleCRDName}, &apiextensionsv1.CustomResourceDefinition{}))

	require.NoError(t, plugin.Uninstall(t.Context(), host))
	assert.Equal(t, 1, h.uninstalls)
	assertConsoleCRDGone(t, kube)

	// Once the uninstall went ahead, nothing puts zot or its console back.
	h.mu.Lock()
	h.status = nil
	h.mu.Unlock()
	require.NoError(t, plugin.Reconcile(t.Context(), host))
	require.NoError(t, plugin.setUpOnce(t.Context(), host, kube))
	assert.Empty(t, h.installs)
	assertConsoleCRDGone(t, kube)
}

func assertConsoleCRDGone(t *testing.T, kube client.Client) {
	t.Helper()
	err := kube.Get(t.Context(), client.ObjectKey{Name: consoleCRDName}, &apiextensionsv1.CustomResourceDefinition{})
	assert.True(t, apierrors.IsNotFound(err), "the console CRD must be gone, got %v", err)
}

func TestUninstallWithoutRelease(t *testing.T) {
	t.Parallel()
	h := &fakeHelm{}
	plugin, _ := newTestPlugin(t, h)

	require.NoError(t, plugin.Uninstall(t.Context(), sdktesting.NewMockHost()))
	assert.Zero(t, h.uninstalls)
}
