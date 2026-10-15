package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestParseReleaseStatus(t *testing.T) {
	// `helm status cnpg -n cnpg-system -o json` from helm 3.16, with manifest
	// and notes cut short. It has no chart block.
	out := []byte(`{"name":"cnpg","info":{"first_deployed":"2026-10-06T12:08:00.586764206Z","last_deployed":"2026-10-12T17:51:04.941537591Z","deleted":"","description":"Upgrade complete","status":"deployed","notes":"..."},"manifest":"...","version":14,"namespace":"cnpg-system"}`)

	status, err := parseReleaseStatus(out)
	require.NoError(t, err)
	assert.Equal(t, "deployed", status.Status)
	assert.Equal(t, 14, status.Revision)
	assert.Empty(t, status.ChartVersion)
	assert.True(t, time.Date(2026, 10, 12, 17, 51, 4, 941537591, time.UTC).Equal(status.LastDeployed))
	assert.True(t, status.Deleted.IsZero())
	assert.False(t, status.Pending())
}

func TestParseReleaseStatusNoTimestamp(t *testing.T) {
	status, err := parseReleaseStatus([]byte(`{"name":"cnpg","info":{"status":"pending-install"},"version":1,"namespace":"cnpg-system"}`))
	require.NoError(t, err)
	assert.True(t, status.LastDeployed.IsZero())
	assert.True(t, status.Deleted.IsZero())
}

func TestParseReleaseStatusUninstalling(t *testing.T) {
	status, err := parseReleaseStatus([]byte(`{"name":"cnpg","info":{"first_deployed":"2026-10-01T08:00:00Z","last_deployed":"2026-10-01T08:00:00Z","deleted":"2026-10-05T09:14:03Z","description":"Deletion in progress (or silently failed)","status":"uninstalling"},"manifest":"...","version":3,"namespace":"cnpg-system"}`))
	require.NoError(t, err)
	assert.Equal(t, "uninstalling", status.Status)
	assert.True(t, time.Date(2026, 10, 5, 9, 14, 3, 0, time.UTC).Equal(status.Deleted))
	assert.False(t, status.Pending())
}

func TestParseReleaseStatusInvalidJSON(t *testing.T) {
	_, err := parseReleaseStatus([]byte("Error: something"))
	assert.Error(t, err)
}

func TestParseChartVersion(t *testing.T) {
	// `helm get metadata cnpg -n cnpg-system -o json` from helm 3.16.
	out := []byte(`{"name":"cnpg","chart":"cloudnative-pg","version":"0.24.0","appVersion":"1.26.0","namespace":"cnpg-system","revision":14,"status":"deployed","deployedAt":"2026-10-12T17:51:04Z"}`)

	version, revision, err := parseChartVersion(out)
	require.NoError(t, err)
	assert.Equal(t, "0.24.0", version)
	assert.Equal(t, 14, revision)
}

// An empty version would read as older than every pin and downgrade a newer
// release, so it is an error rather than "".
func TestParseChartVersionMissing(t *testing.T) {
	_, _, err := parseChartVersion([]byte(`{"name":"cnpg","namespace":"cnpg-system"}`))
	assert.Error(t, err)
}

func TestReleaseStatusPending(t *testing.T) {
	for _, s := range []string{"pending-install", "pending-upgrade", "pending-rollback"} {
		assert.True(t, (&ReleaseStatus{Status: s}).Pending(), s)
	}
	for _, s := range []string{"deployed", "failed", "superseded"} {
		assert.False(t, (&ReleaseStatus{Status: s}).Pending(), s)
	}
}

func TestIsReleaseNotFound(t *testing.T) {
	assert.True(t, isReleaseNotFound("Error: release: not found"))
	assert.False(t, isReleaseNotFound(`Error: secrets is forbidden: User "x" cannot list resource "secrets"`))
}
