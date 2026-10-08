package cnpg

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func TestInstallVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current *helm.ReleaseStatus
		want    string
	}{
		{"absent", nil, Version},
		{"deployed at the same version", &helm.ReleaseStatus{Status: "deployed", ChartVersion: Version}, Version},
		// Another plugin, built later, installed a newer chart: keep it.
		{"deployed newer", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "99.0.1"}, "99.0.1"},
		{"deployed older", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "0.1.2"}, Version},
		// Semver, not string order: 0.9 is older than 0.24.
		{"deployed older, one-digit minor", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "0.9.0"}, Version},
		{"failed", &helm.ReleaseStatus{Status: "failed", ChartVersion: Version}, Version},
		// A newer plugin's upgrade failed: retry its version, never downgrade.
		{"failed newer", &helm.ReleaseStatus{Status: "failed", ChartVersion: "99.0.0"}, "99.0.0"},
		{"uninstalled newer", &helm.ReleaseStatus{Status: "uninstalled", ChartVersion: "99.0.0"}, "99.0.0"},
		{"failed older", &helm.ReleaseStatus{Status: "failed", ChartVersion: "0.1.0"}, Version},
		{"unparseable version", &helm.ReleaseStatus{Status: "failed", ChartVersion: "latest"}, Version},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := installVersion(tt.current, now)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInstallVersionPendingStale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status       string
		lastDeployed time.Time
		recovery     string
	}{
		{"pending-upgrade", now.Add(-time.Hour), "helm -n cnpg-system rollback cnpg"},
		{"pending-rollback", now.Add(-time.Hour), "helm -n cnpg-system rollback cnpg"},
		// A first install has no earlier revision to roll back to.
		{"pending-install", now.Add(-time.Hour), "helm -n cnpg-system uninstall cnpg"},
		// No timestamp: cannot tell it is recent, so give the way out.
		{"pending-install", time.Time{}, "helm -n cnpg-system uninstall cnpg"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			_, err := installVersion(&helm.ReleaseStatus{Status: tt.status, ChartVersion: Version, LastDeployed: tt.lastDeployed}, now)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrInProgress)
			assert.Contains(t, err.Error(), tt.status)
			assert.Contains(t, err.Error(), tt.recovery)
		})
	}
}

// Another plugin's install still in --wait: report it as in progress, and do
// not tell the admin to uninstall it.
func TestInstallVersionPendingRecent(t *testing.T) {
	t.Parallel()
	_, err := installVersion(&helm.ReleaseStatus{Status: "pending-install", ChartVersion: Version, LastDeployed: now.Add(-2 * time.Minute)}, now)
	require.ErrorIs(t, err, ErrInProgress)
	assert.Contains(t, err.Error(), "pending-install")
	assert.NotContains(t, err.Error(), "uninstall")
}

func TestOperatorPresent(t *testing.T) {
	t.Parallel()
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "cnpg-cloudnative-pg",
		Namespace: Namespace,
		Labels:    map[string]string{"app.kubernetes.io/name": Chart, "app.kubernetes.io/instance": Release},
	}}
	crds := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: CRDNames[0]}}
	tests := []struct {
		name    string
		objects []client.Object
		want    bool
	}{
		{"all there", []client.Object{deployment, crds}, true},
		{"deployment deleted", []client.Object{crds}, false},
		{"CRD deleted", []client.Object{deployment}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kube := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(tt.objects...).Build()
			got, err := operatorPresent(context.Background(), kube)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	return scheme
}
