package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvoyGatewayChartSpec(t *testing.T) {
	installer := newEnvoyGatewayInstaller(pluginConfig{
		EnvoyGatewayVersion: "v1.8.3",
		GatewayNamespace:    "envoy-gateway-system",
	})

	spec := installer.chart()
	require.Equal(t, "eg", spec.releaseName)
	require.Equal(t, "oci://docker.io/envoyproxy/gateway-helm", spec.chartRef)
	require.Equal(t, "v1.8.3", spec.version)
}
