package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("FUNP_ROOK_CHART_VERSION", "")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "v1.16.0", cfg.RookChartVersion)
	assert.Equal(t, "rook-ceph", cfg.RookNamespace)
	assert.Equal(t, "rook-ceph", cfg.ClusterNamespace)
}

func TestLoadConfigOverride(t *testing.T) {
	t.Setenv("FUNP_ROOK_CHART_VERSION", "v1.16.1")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "v1.16.1", cfg.RookChartVersion)
}
