package v1_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
)

// startEnvtest runs a real kube-apiserver with the generated CRD installed, so
// the CEL validation rules and defaulting are exercised exactly as a cluster
// evaluates them. Skipped when no envtest assets are installed (run
// `go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.23 use`
// once to install them).
func startEnvtest(t *testing.T) client.Client {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		out, err := exec.CommandContext(t.Context(), "go", "run",
			"sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.23",
			"use", "-i", "-p", "path").Output()
		if err != nil || len(out) == 0 {
			t.Skip("no envtest assets installed; run setup-envtest use")
		}
		t.Setenv("KUBEBUILDER_ASSETS", strings.TrimSpace(string(out)))
	}

	env := &envtest.Environment{CRDDirectoryPaths: []string{"../../chart/crds"}}
	cfg, err := env.Start()
	require.NoError(t, err)
	t.Cleanup(func() { _ = env.Stop() })

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openfscv1.AddToScheme(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	require.NoError(t, c.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}}))
	return c
}

func selfSpec() *openfscv1.FSCInstallationSpec {
	return &openfscv1.FSCInstallationSpec{
		GroupID:   "fsc-demo",
		PeerID:    "12345678901234567899",
		Directory: openfscv1.DirectoryConfig{Mode: openfscv1.DirectoryModeSelf},
		Postgres:  openfscv1.PostgresConfig{StorageClass: "local-path"},
	}
}

func externalSpec() *openfscv1.FSCInstallationSpec {
	spec := selfSpec()
	spec.Directory = openfscv1.DirectoryConfig{
		Mode: openfscv1.DirectoryModeExternal,
		External: &openfscv1.ExternalDirectory{
			Address:     "https://directory.example.com:8443",
			PeerID:      "98765432109876543210",
			TrustAnchor: openfscv1.SecretKeySelector{Name: "group-trust"},
		},
	}
	spec.Certificate = &openfscv1.CertificateRef{ExistingSecret: "org-cert"}
	return spec
}

func installation(name string, spec *openfscv1.FSCInstallationSpec) *openfscv1.FSCInstallation {
	return &openfscv1.FSCInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
		Spec:       *spec,
	}
}

func TestFSCInstallationValidation(t *testing.T) {
	c := startEnvtest(t)
	ctx := context.Background()

	t.Run("self mode defaults", func(t *testing.T) {
		inst := installation("self-defaults", selfSpec())
		require.NoError(t, c.Create(ctx, inst))
		require.Equal(t, int32(1), inst.Spec.Postgres.Instances)
		require.Equal(t, "1Gi", inst.Spec.Postgres.StorageSize)
		require.Equal(t, []string{"servicePublication", "delegatedServicePublication"}, inst.Spec.AutoSignGrants)
	})

	t.Run("external mode defaults trust anchor key", func(t *testing.T) {
		inst := installation("external-defaults", externalSpec())
		require.NoError(t, c.Create(ctx, inst))
		require.Equal(t, "ca.crt", inst.Spec.Directory.External.TrustAnchor.Key)
	})

	invalid := []struct {
		name    string
		mutate  func(*openfscv1.FSCInstallationSpec)
		message string
	}{
		{
			name:    "external mode requires the external block",
			mutate:  func(s *openfscv1.FSCInstallationSpec) { *s = *externalSpec(); s.Directory.External = nil },
			message: "external is required for mode External",
		},
		{
			name:    "self mode forbids the external block",
			mutate:  func(s *openfscv1.FSCInstallationSpec) { s.Directory.External = externalSpec().Directory.External },
			message: "forbidden for mode Self",
		},
		{
			name:    "external mode requires a certificate",
			mutate:  func(s *openfscv1.FSCInstallationSpec) { *s = *externalSpec(); s.Certificate = nil },
			message: "certificate is required for directory.mode External",
		},
		{
			name:    "self mode forbids a certificate",
			mutate:  func(s *openfscv1.FSCInstallationSpec) { s.Certificate = &openfscv1.CertificateRef{ExistingSecret: "x"} },
			message: "forbidden for Self",
		},
		{
			name: "self mode forbids inway certificate overrides",
			mutate: func(s *openfscv1.FSCInstallationSpec) {
				s.Inways = []openfscv1.InwayConfig{{Name: "edge", Certificate: &openfscv1.CertificateRef{ExistingSecret: "x"}}}
			},
			message: "inway certificate overrides are only valid for directory.mode External",
		},
		{
			name: "gateway names must be DNS labels",
			mutate: func(s *openfscv1.FSCInstallationSpec) {
				s.Inways = []openfscv1.InwayConfig{{Name: "Bad_Name"}}
			},
			message: "should match",
		},
		{
			name: "gateway names must be unique",
			mutate: func(s *openfscv1.FSCInstallationSpec) {
				s.Outways = []openfscv1.OutwayConfig{{Name: "dup"}, {Name: "dup"}}
			},
			message: "Duplicate value",
		},
		{
			name:    "postgres storageClass is required",
			mutate:  func(s *openfscv1.FSCInstallationSpec) { s.Postgres.StorageClass = "" },
			message: "spec.postgres.storageClass",
		},
		{
			name:    "controllerURL must be http(s)",
			mutate:  func(s *openfscv1.FSCInstallationSpec) { s.ControllerURL = "javascript:alert(1)" },
			message: "should match",
		},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			spec := selfSpec()
			tc.mutate(spec)
			err := c.Create(ctx, installation("invalid", spec))
			require.ErrorContains(t, err, tc.message)
		})
	}

	t.Run("groupID is immutable", func(t *testing.T) {
		inst := installation("immutable", selfSpec())
		require.NoError(t, c.Create(ctx, inst))

		inst.Spec.PeerID = "11111111111111111111"
		require.NoError(t, c.Update(ctx, inst), "peerID must stay mutable")

		inst.Spec.GroupID = "another-group"
		require.ErrorContains(t, c.Update(ctx, inst), "groupID is immutable")
	})
}
