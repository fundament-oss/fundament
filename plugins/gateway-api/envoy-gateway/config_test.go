package main

import (
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/require"
)

func TestPluginConfigDefaults(t *testing.T) {
	var cfg pluginConfig
	require.NoError(t, env.Parse(&cfg))

	require.Equal(t, "v1.8.3", cfg.EnvoyGatewayVersion)
	require.Equal(t, "envoy-gateway-system", cfg.GatewayNamespace)
	require.Equal(t, "eg", cfg.GatewayClassName)
}
