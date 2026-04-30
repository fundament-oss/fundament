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

	assert.Equal(t, "external-dns", def.Metadata.Name)
	assert.Equal(t, "External DNS", def.Metadata.DisplayName)
	assert.Equal(t, "v1.16.1", def.Metadata.Version)
	assert.Equal(t, "Fundament", def.Metadata.Author)
	assert.Equal(t, "Apache-2.0", def.Metadata.License)
	assert.Equal(t, "rectangle-stack", def.Metadata.Icon)
	assert.Equal(t, "https://kubernetes-sigs.github.io/external-dns/", def.Metadata.URLs.Homepage)
	assert.NotEmpty(t, def.Spec.Permissions.Capabilities)
	assert.NotEmpty(t, def.Spec.Permissions.RBAC)
	assert.Empty(t, def.Spec.Menu.Organization)
	assert.Len(t, def.Spec.Menu.Project, 1)
}

func TestNewExternalDNSPlugin(t *testing.T) {
	def := pluginruntime.PluginDefinition{
		Metadata: pluginruntime.PluginMetadata{
			Name:    "external-dns",
			Version: "v1.16.1",
		},
	}

	plugin := NewExternalDNSPlugin(&def)
	assert.Equal(t, def, plugin.Definition())
}

func TestDefinitionFields(t *testing.T) {
	def, err := pluginruntime.LoadDefinition("definition.yaml")
	require.NoError(t, err)

	plugin := NewExternalDNSPlugin(&def)
	got := plugin.Definition()

	assert.Equal(t, "external-dns", got.Metadata.Name)
	assert.Equal(t, "External DNS", got.Metadata.DisplayName)
	assert.Equal(t, "v1.16.1", got.Metadata.Version)
}

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	def := pluginruntime.PluginDefinition{}
	plugin := NewExternalDNSPlugin(&def)

	// Verify all expected interfaces are implemented.
	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Installer = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin
}
