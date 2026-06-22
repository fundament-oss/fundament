package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
	"github.com/fundament-oss/fundament/openfsc-operator/charts"
	"github.com/fundament-oss/fundament/openfsc-operator/internal/helm"
)

func selfInstallation() *openfscv1.FSCInstallation {
	return &openfscv1.FSCInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "team-a"},
		Spec: openfscv1.FSCInstallationSpec{
			GroupID:        "fsc-demo",
			PeerID:         "12345678901234567899",
			Directory:      openfscv1.DirectoryConfig{Mode: openfscv1.DirectoryModeSelf},
			AutoSignGrants: []string{"servicePublication", "delegatedServicePublication"},
		},
	}
}

func externalInstallation() *openfscv1.FSCInstallation {
	inst := selfInstallation()
	inst.Spec.Directory = openfscv1.DirectoryConfig{
		Mode: openfscv1.DirectoryModeExternal,
		External: &openfscv1.ExternalDirectory{
			Address:     "https://directory.example.com:8443",
			PeerID:      "98765432109876543210",
			TrustAnchor: openfscv1.SecretKeySelector{Name: "group-trust", Key: "ca.crt"},
		},
	}
	inst.Spec.Certificate = &openfscv1.CertificateRef{ExistingSecret: "team-a-group-cert"} //nolint:gosec // Secret resource name, not a credential
	return inst
}

// renderChart runs the client-side equivalent of `helm template` so the values
// builders are validated against the real vendored charts (including their
// values schemas) without a cluster.
func renderChart(t *testing.T, chrt *chart.Chart, release string, values map[string]any) string {
	t.Helper()
	install := action.NewInstall(new(action.Configuration))
	install.DryRun = true
	install.ClientOnly = true
	install.ReleaseName = release
	install.Namespace = "team-a"
	rel, err := install.Run(chrt, values)
	require.NoError(t, err)
	return rel.Manifest
}

func loadUmbrella(t *testing.T) *chart.Chart {
	t.Helper()
	archive, err := charts.FS.ReadFile(charts.UmbrellaArchive)
	require.NoError(t, err)
	chrt, err := helm.LoadArchive(archive)
	require.NoError(t, err)
	return chrt
}

func TestCoreValuesRenderSelf(t *testing.T) {
	manifest := renderChart(t, loadUmbrella(t), umbrellaRelease, coreValues(selfInstallation()))

	for _, want := range []string{
		"name: " + managerDeployment,
		"name: " + controllerDeployment,
		"name: " + managerExternalService,
		"name: " + managerInternalSecret,
		"name: " + controllerInternalSecret,
		"name: " + internalCASecret,
		"name: " + internalIssuer,
		"name: " + postgresSecret,
		groupCASecret,
		managerGroupCertSecret,
		"https://fsc-open-fsc-manager-external.team-a:8443",
	} {
		require.Contains(t, manifest, want)
	}

	require.NotContains(t, manifest, "kind: Ingress", "Controller UI ingress must stay disabled")
	require.NotContains(t, manifest, "kind: HTTPRoute")
	require.NotContains(t, manifest, "name: fsc-open-fsc-inway", "bundled gateways must stay disabled")
	require.NotContains(t, manifest, "name: fsc-open-fsc-outway", "bundled gateways must stay disabled")
}

func TestCoreValuesRenderExternal(t *testing.T) {
	manifest := renderChart(t, loadUmbrella(t), umbrellaRelease, coreValues(externalInstallation()))

	for _, want := range []string{
		"https://directory.example.com:8443",
		"team-a-group-cert",
		"group-trust",
	} {
		require.Contains(t, manifest, want)
	}
	require.NotContains(t, manifest, groupCASecret, "External mode must not reference the self-signed group CA")
	require.NotContains(t, manifest, managerGroupCertSecret, "External mode must not reference the minted Manager certificate")
}

func TestInwayValuesRender(t *testing.T) {
	chrt, err := helm.LoadDir(charts.FS, charts.InwayDir)
	require.NoError(t, err)

	for name, inst := range map[string]*openfscv1.FSCInstallation{"self": selfInstallation(), "external": externalInstallation()} {
		t.Run(name, func(t *testing.T) {
			gw := openfscv1.InwayConfig{Name: "default"}
			values, err := helm.SetValues(inwayValues(inst, gw))
			require.NoError(t, err)
			manifest := renderChart(t, chrt, inwayRelease(gw.Name), values)

			require.Contains(t, manifest, "name: fsc-inway-default")
			require.Contains(t, manifest, "https://fsc-inway-default.team-a:443")
			if inst.Spec.Directory.External != nil {
				require.Contains(t, manifest, "team-a-group-cert")
			} else {
				require.Contains(t, manifest, certSecret("fsc-inway-default", "group"))
			}
		})
	}
}

func TestOutwayValuesRender(t *testing.T) {
	chrt, err := helm.LoadDir(charts.FS, charts.OutwayDir)
	require.NoError(t, err)

	inst := selfInstallation()
	gw := openfscv1.OutwayConfig{Name: "default", Certificate: nil}
	values, err := helm.SetValues(outwayValues(inst, gw))
	require.NoError(t, err)
	manifest := renderChart(t, chrt, outwayRelease(gw.Name), values)

	require.Contains(t, manifest, "name: fsc-outway-default")
	require.Contains(t, manifest, certSecret("fsc-outway-default", "internal"))
}

func TestGatewayCertificateOverride(t *testing.T) {
	inst := externalInstallation()
	gw := openfscv1.InwayConfig{Name: "edge", Certificate: &openfscv1.CertificateRef{ExistingSecret: "edge-cert"}}
	require.Equal(t, "edge-cert", inwayValues(inst, gw)["certificates.group.existingSecret"])

	gw.Certificate = nil
	require.Equal(t, "team-a-group-cert", inwayValues(inst, gw)["certificates.group.existingSecret"])
}

// Kubernetes object and DNS label names cap at 63 characters; the longest
// derived names must stay inside that with the CRD's 30-char gateway cap.
func TestNameLengthBudget(t *testing.T) {
	longest := strings.Repeat("x", 30)
	for _, name := range []string{
		managerUnauthSecret,
		certSecret(inwayRelease(longest), "internal"),
		certSecret(outwayRelease(longest), "internal"),
		certName(inwayRelease(longest), "internal"),
		postgresService,
	} {
		require.LessOrEqual(t, len(name), 63, name)
	}
}
