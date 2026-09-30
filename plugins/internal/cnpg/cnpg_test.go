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
	tests := []struct {
		status   string
		recovery string
	}{
		{"pending-upgrade", "helm -n cnpg-system rollback cnpg"},
		{"pending-rollback", "helm -n cnpg-system rollback cnpg"},
		// A first install has no earlier revision to roll back to.
		{"pending-install", "helm -n cnpg-system uninstall cnpg"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			install, err := needsInstall(&helm.ReleaseStatus{Status: tt.status, ChartVersion: "0.24.0"}, "0.24.0")
			require.Error(t, err)
			assert.False(t, install)
			assert.Contains(t, err.Error(), tt.status)
			assert.Contains(t, err.Error(), tt.recovery)
		})
	}
}
