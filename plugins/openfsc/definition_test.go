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
	assert.Equal(t, "v4.0.0", def.Metadata.Version)
	assert.Equal(t, "Fundament", def.Metadata.Author)
	assert.Equal(t, "AGPL-3.0", def.Metadata.License)
	assert.NotEmpty(t, def.Spec.Permissions.Capabilities)
	assert.NotEmpty(t, def.Spec.Permissions.RBAC)

	// The console renders project menu entries only.
	assert.Empty(t, def.Spec.Menu.Organization)
	assert.Len(t, def.Spec.Menu.Project, 1)
	assert.Equal(t, "fscinstallations.openfsc.fundament.io", def.Spec.Menu.Project[0].CRD)
	assert.True(t, def.Spec.Menu.Project[0].Create)

	assert.Equal(t, []string{"fscinstallations.openfsc.fundament.io"}, def.Spec.CRDs)
	assert.Len(t, def.Spec.CustomComponents, 1)
	assert.Equal(t, "fscinstallations-list.html", def.Spec.CustomComponents["FSCInstallation"].List)
	assert.Equal(t, "fscinstallations-detail.html", def.Spec.CustomComponents["FSCInstallation"].Detail)
	assert.Equal(t, "fscinstallations-create.html", def.Spec.CustomComponents["FSCInstallation"].Create)

	require.Len(t, def.Spec.AllowedResources, 1)
	assert.Equal(t, "fscinstallations", def.Spec.AllowedResources[0].Resource)
	assert.ElementsMatch(t, []string{"list", "get", "create"}, def.Spec.AllowedResources[0].Verbs)
}

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	def := pluginruntime.PluginDefinition{}
	plugin, err := NewOpenFSCPlugin(&def)
	require.NoError(t, err)

	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Installer = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin
}

func TestNewOpenFSCPluginDefaults(t *testing.T) {
	def := pluginruntime.PluginDefinition{}
	plugin, err := NewOpenFSCPlugin(&def)
	require.NoError(t, err)

	assert.Equal(t, "ghcr.io/fundament-oss/fundament/openfsc-operator:latest", plugin.cfg.OperatorImage)
}
