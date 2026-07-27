package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	plugin, err := NewOpenFSCPlugin()
	require.NoError(t, err)

	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Installer = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin

	assert.NotNil(t, plugin)
}

func TestNewOpenFSCPluginDefaults(t *testing.T) {
	plugin, err := NewOpenFSCPlugin()
	require.NoError(t, err)

	assert.Equal(t, "ghcr.io/fundament-oss/fundament/openfsc-operator:latest", plugin.cfg.OperatorImage)
}
