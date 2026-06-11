package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestLoadDefinition(t *testing.T) {
	def, err := pluginruntime.LoadDefinition("definition.yaml")
	require.NoError(t, err)

	assert.Equal(t, "openfsc", def.Metadata.Name)
	assert.Equal(t, "OpenFSC", def.Metadata.DisplayName)
	assert.Equal(t, "v3.0.0", def.Metadata.Version)
	assert.Equal(t, "Fundament", def.Metadata.Author)
	assert.Equal(t, "EUPL-1.2", def.Metadata.License)
	assert.NotEmpty(t, def.Spec.Permissions.Capabilities)
	assert.NotEmpty(t, def.Spec.Permissions.RBAC)
	assert.Len(t, def.Spec.Menu.Organization, 3)
	assert.ElementsMatch(t, []string{
		"directories.openfsc.fundament.io",
		"peers.openfsc.fundament.io",
		"inways.openfsc.fundament.io",
		"outways.openfsc.fundament.io",
	}, def.Spec.CRDs)
	assert.Len(t, def.Spec.CustomComponents, 3)
}

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	def := pluginruntime.PluginDefinition{}
	plugin, err := NewOpenFSCPlugin(&def)
	require.NoError(t, err)

	// Verify all expected interfaces are implemented.
	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Installer = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin
}

func TestNewOpenFSCPluginDefaults(t *testing.T) {
	def := pluginruntime.PluginDefinition{}
	plugin, err := NewOpenFSCPlugin(&def)
	require.NoError(t, err)

	assert.Equal(t, "fsc-demo", plugin.cfg.GroupID)
	assert.Equal(t, "12345678901234567899", plugin.cfg.DirectoryPeerID)
	assert.Equal(t, "fsc", plugin.cfg.Namespace)
	assert.Equal(t, "ghcr.io/fundament-oss/fundament/openfsc-operator:latest", plugin.cfg.OperatorImage)
}
