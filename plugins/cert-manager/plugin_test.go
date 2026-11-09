package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	plugin := NewCertManagerPlugin()

	// Verify all expected interfaces are implemented.
	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Installer = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin

	assert.NotNil(t, plugin)
}

func TestChartValues(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]string{"crds.enabled": "true"}, chartValues(false))
	assert.Equal(t, map[string]string{
		"crds.enabled":              "true",
		"config.gatewayAPI.enabled": "true",
	}, chartValues(true))
}

func TestParseGatewayAPIEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config string
		want   bool
	}{
		{name: "no config map", config: "", want: false},
		{name: "enabled", config: "apiVersion: controller.config.cert-manager.io/v1alpha1\ngatewayAPI:\n  enabled: true\nkind: ControllerConfiguration\n", want: true},
		{name: "disabled", config: "gatewayAPI:\n  enabled: false\n", want: false},
		{name: "other settings only", config: "kind: ControllerConfiguration\nlogging:\n  verbosity: 2\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseGatewayAPIEnabled(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := parseGatewayAPIEnabled("gatewayAPI: [")
	require.Error(t, err)
}
