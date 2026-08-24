package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRBACForbidden(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "secrets forbidden during install",
			output: `Error: query: failed to query with labels: secrets is forbidden: User "system:serviceaccount:plugin-cert-manager:plugin-cert-manager" cannot list resource "secrets" in API group "" in the namespace "cert-manager"`,
			want:   true,
		},
		{
			name:   "chart not found is not an RBAC error",
			output: `Error: failed to download "cert-manager" (hint: running helm repo update may help)`,
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRBACForbidden(tc.output))
		})
	}
}

func TestOCIInstallArgs(t *testing.T) {
	c := NewClient("envoy-gateway-system")
	args := c.ociInstallArgs("eg", "oci://docker.io/envoyproxy/gateway-helm", "v1.8.3",
		map[string]string{"b": "2", "a": "1"})

	assert.Equal(t, []string{
		"upgrade", "--install", "eg", "oci://docker.io/envoyproxy/gateway-helm",
		"--namespace", "envoy-gateway-system", "--create-namespace", "--wait",
		"--version", "v1.8.3",
		"--set", "a=1", "--set", "b=2",
	}, args)
}

func TestOCIInstallArgsNoVersion(t *testing.T) {
	c := NewClient("ns")
	args := c.ociInstallArgs("r", "oci://example/chart", "", nil)
	assert.NotContains(t, args, "--version")
}
