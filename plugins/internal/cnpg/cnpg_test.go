package cnpg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

func TestNeedsInstall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current *helm.ReleaseStatus
		want    bool
	}{
		{"absent", nil, true},
		{"deployed at the same version", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "0.24.0"}, false},
		// Another plugin, built later, installed a newer chart: leave it.
		{"deployed newer", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "0.25.1"}, false},
		{"deployed older", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "0.23.2"}, true},
		// Semver, not string order: 0.9 is older than 0.24.
		{"deployed older, one-digit minor", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "0.9.0"}, true},
		{"failed", &helm.ReleaseStatus{Status: "failed", ChartVersion: "0.24.0"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := needsInstall(tt.current, "0.24.0")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNeedsInstallPending(t *testing.T) {
	t.Parallel()
	install, err := needsInstall(&helm.ReleaseStatus{Status: "pending-upgrade", ChartVersion: "0.24.0"}, "0.24.0")
	require.Error(t, err)
	assert.False(t, install)
	assert.Contains(t, err.Error(), "pending-upgrade")
	assert.Contains(t, err.Error(), "helm -n cnpg-system rollback cnpg")
}
