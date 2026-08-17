package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "rook-ceph", cfg.RookNamespace)
	assert.Equal(t, "rook-ceph", cfg.ClusterNamespace)
}

func TestLoadConfigOverride(t *testing.T) {
	t.Setenv("FUNP_CLUSTER_NAMESPACE", "ceph-storage")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "ceph-storage", cfg.ClusterNamespace)
	assert.Equal(t, "rook-ceph", cfg.RookNamespace, "the two namespaces are configured independently")
}

// Defaulting this on would waive Rook's version guard for every install to spare
// one development platform.
func TestAllowUnsupportedCephDefaultsOff(t *testing.T) {
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.AllowUnsupportedCeph)
}

func TestAllowUnsupportedCephOverride(t *testing.T) {
	t.Setenv("FUNP_ALLOW_UNSUPPORTED_CEPH", "true")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.True(t, cfg.AllowUnsupportedCeph)
}

// An env var would let the same consented image pull a different Rook, which is
// what the manifest hash exists to prevent.
func TestRookChartVersionIsNotConfigurable(t *testing.T) {
	t.Setenv("FUNP_ROOK_CHART_VERSION", "v1.99.0")
	_, err := LoadConfig()
	require.NoError(t, err, "an unknown FUNP_ var must not break config parsing")
	assert.Equal(t, "v1.16.0", rookChartVersion)
}

// If these drift, the sandbox instructions stop reproducing what the smoke
// script proves.
func TestDevConfigMatchesSmokeScript(t *testing.T) {
	smoke, err := os.ReadFile("../../../deploy/k3d/rook-smoke.sh")
	require.NoError(t, err)
	assert.Contains(t, string(smoke), "ROOK_VERSION:-"+rookChartVersion,
		"rook-smoke.sh must install the same Rook chart version the plugin pins")
}
