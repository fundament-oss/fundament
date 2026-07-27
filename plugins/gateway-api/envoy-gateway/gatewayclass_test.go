package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestBuildGatewayClass(t *testing.T) {
	cfg := pluginConfig{GatewayClassName: "eg"}

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal(buildGatewayClass(cfg), &parsed))

	require.Equal(t, "GatewayClass", parsed["kind"])
	require.Equal(t, "eg", parsed["metadata"].(map[string]any)["name"])
	require.Equal(t, "gateway.envoyproxy.io/gatewayclass-controller",
		parsed["spec"].(map[string]any)["controllerName"])
}
