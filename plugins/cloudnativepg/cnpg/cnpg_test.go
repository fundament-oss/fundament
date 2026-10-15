package cnpg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
		// A newer plugin installed a newer chart before a rollback: keep it.
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
		{"pending-upgrade", now.Add(-time.Hour), "helm -n cnpg-system rollback fundament-cloudnativepg"},
		{"pending-rollback", now.Add(-time.Hour), "helm -n cnpg-system rollback fundament-cloudnativepg"},
		// A first install has no earlier revision to roll back to.
		{"pending-install", now.Add(-time.Hour), "helm -n cnpg-system uninstall fundament-cloudnativepg"},
		// No timestamp: cannot tell it is recent, so give the way out.
		{"pending-install", time.Time{}, "helm -n cnpg-system uninstall fundament-cloudnativepg"},
		// An interrupted uninstall is finished, not undone.
		{"uninstalling", now.Add(-time.Hour), "helm -n cnpg-system uninstall fundament-cloudnativepg"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			current := &helm.ReleaseStatus{Status: tt.status, ChartVersion: Version, LastDeployed: tt.lastDeployed, Deleted: tt.lastDeployed}
			_, err := installVersion(current, now)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrInProgress)
			assert.Contains(t, err.Error(), tt.status)
			assert.Contains(t, err.Error(), tt.recovery)
		})
	}
}

// An earlier pod's install still in --wait: report it as in progress, and do
// not tell the admin to uninstall it.
func TestInstallVersionPendingRecent(t *testing.T) {
	t.Parallel()
	_, err := installVersion(&helm.ReleaseStatus{Status: "pending-install", ChartVersion: Version, LastDeployed: now.Add(-2 * time.Minute)}, now)
	require.ErrorIs(t, err, ErrInProgress)
	assert.Contains(t, err.Error(), "pending-install")
	assert.NotContains(t, err.Error(), "uninstall")
}

// An admin's uninstall still running: wait for it rather than upgrade over it.
// The deploy time is old; the uninstall's own start time is what counts.
func TestInstallVersionUninstallingRecent(t *testing.T) {
	t.Parallel()
	current := &helm.ReleaseStatus{Status: "uninstalling", ChartVersion: Version, LastDeployed: now.Add(-30 * 24 * time.Hour), Deleted: now.Add(-time.Minute)}
	_, err := installVersion(current, now)
	require.ErrorIs(t, err, ErrInProgress)
	assert.Contains(t, err.Error(), "uninstalling")
}

func TestConcurrentOperation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		output string
		want   bool
	}{
		{"Error: UPGRADE FAILED: another operation (install/upgrade/rollback) is in progress", true},
		{"Error: INSTALLATION FAILED: cannot re-use a name that is still in use", true},
		{`Error: failed to download "cloudnative-pg"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			t.Parallel()
			err := errors.New("helm install failed: " + tt.output + ": exit status 1")
			assert.Equal(t, tt.want, concurrentOperation(err))
		})
	}
}

func TestOperatorPresent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		objects []client.Object
		want    bool
	}{
		{"all there", []client.Object{ourDeployment(), ourCRD(CRDNames[0]), ourCRD(CRDNames[1])}, true},
		{"deployment deleted", []client.Object{ourCRD(CRDNames[0]), ourCRD(CRDNames[1])}, false},
		{"Cluster CRD deleted", []client.Object{ourDeployment(), ourCRD(CRDNames[1])}, false},
		{"catalog CRD deleted", []client.Object{ourDeployment(), ourCRD(CRDNames[0])}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kube := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(tt.objects...).Build()
			got, err := OperatorPresent(context.Background(), kube)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestForeignOwner(t *testing.T) {
	t.Parallel()
	openfscCRD := ourCRD(CRDNames[0])
	openfscCRD.Annotations = map[string]string{"meta.helm.sh/release-name": "cnpg", "meta.helm.sh/release-namespace": "cnpg-system"}
	kubectlCRD := ourCRD(CRDNames[0])
	kubectlCRD.Annotations = nil
	otherOperator := deployment("cnpg-system", "cnpg-controller-manager", "ghcr.io/cloudnative-pg/cloudnative-pg:1.25.1")
	tests := []struct {
		name    string
		objects []client.Object
		want    string
	}{
		{"nothing installed", nil, ""},
		{"ours", []client.Object{ourCRD(CRDNames[0]), ourDeployment()}, ""},
		// Kept from our own uninstall: a reinstall adopts it.
		{"our CRD without operator", []client.Object{ourCRD(CRDNames[0])}, ""},
		{"openfsc's release", []client.Object{openfscCRD}, "CRD clusters.postgresql.cnpg.io belongs to Helm release cnpg-system/cnpg"},
		{"plain manifests", []client.Object{kubectlCRD}, "CRD clusters.postgresql.cnpg.io was not installed with Helm"},
		// kubectl apply of the manifests over our CRDs keeps our annotations,
		// but adds a second operator.
		{"second operator", []client.Object{ourCRD(CRDNames[0]), ourDeployment(), otherOperator}, "Deployment cnpg-system/cnpg-controller-manager runs the CloudNativePG operator"},
		{"unrelated deployment", []client.Object{ourCRD(CRDNames[0]), deployment("team-a", "web", "nginx:1.27")}, ""},
		// Helm checks every CRD of the chart, so the plugin does too.
		{"another CRD of the chart", []client.Object{ourCRD(CRDNames[0]), otherReleaseCRD("poolers.postgresql.cnpg.io")}, "CRD poolers.postgresql.cnpg.io belongs to Helm release cnpg-system/cnpg"},
		{"unrelated CRD", []client.Object{ourCRD(CRDNames[0]), otherReleaseCRD("certificates.cert-manager.io")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kube := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(tt.objects...).Build()
			got, err := ForeignOwner(context.Background(), kube)
			require.NoError(t, err)
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			// Followed by how to hand the operator over.
			assert.True(t, strings.HasPrefix(got, tt.want+". To hand the operator to this plugin, "), got)
		})
	}
}

// The hand-over for kept CRDs names the commands, since the plugin status is
// all an organization admin sees in the console.
func TestForeignOwnerHandOver(t *testing.T) {
	t.Parallel()
	openfscCRD := ourCRD(CRDNames[0])
	openfscCRD.Annotations = map[string]string{"meta.helm.sh/release-name": "cnpg", "meta.helm.sh/release-namespace": "cnpg-system"}
	kube := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(openfscCRD).Build()

	got, err := ForeignOwner(context.Background(), kube)
	require.NoError(t, err)
	assert.Contains(t, got, "`helm -n cnpg-system uninstall cnpg`")
	assert.Contains(t, got, "app.kubernetes.io/managed-by=Helm")
	assert.Contains(t, got, "meta.helm.sh/release-name=fundament-cloudnativepg meta.helm.sh/release-namespace=cnpg-system")
}

func TestIsOperatorImage(t *testing.T) {
	t.Parallel()
	for image, want := range map[string]bool{
		"ghcr.io/cloudnative-pg/cloudnative-pg:1.26.0":      true,
		"ghcr.io/cloudnative-pg/cloudnative-pg@sha256:0123": true,
		"registry.local:5000/mirror/cloudnative-pg:1.26.0":  true,
		"cloudnative-pg": true,
		"ghcr.io/cloudnative-pg/postgresql:17.11-202610120828-standard-bookworm": false,
		"registry.local:5000/cloudnative-pg-plugin:v1":                           false,
	} {
		assert.Equal(t, want, isOperatorImage(image), image)
	}
}

func otherReleaseCRD(name string) *apiextensionsv1.CustomResourceDefinition {
	crd := ourCRD(name)
	crd.Annotations = map[string]string{"meta.helm.sh/release-name": "cnpg", "meta.helm.sh/release-namespace": "cnpg-system"}
	return crd
}

func ourCRD(name string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name:        name,
		Annotations: map[string]string{"meta.helm.sh/release-name": Release, "meta.helm.sh/release-namespace": Namespace},
	}}
}

func ourDeployment() *appsv1.Deployment {
	d := deployment(Namespace, Release+"-cloudnative-pg", "ghcr.io/cloudnative-pg/cloudnative-pg:1.26.0")
	d.Labels = map[string]string{"app.kubernetes.io/name": Chart, "app.kubernetes.io/instance": Release}
	return d
}

func deployment(namespace, name, image string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "manager", Image: image}},
		}}},
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	return scheme
}
