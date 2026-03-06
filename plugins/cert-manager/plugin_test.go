package main

import (
	"testing"

	pluginsdk "github.com/fundament-oss/fundament/plugin-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefinition(t *testing.T) {
	def, err := pluginsdk.LoadDefinition("definition.yaml")
	require.NoError(t, err)

	assert.Equal(t, "cert-manager", def.Metadata.Name)
	assert.Equal(t, "Cert Manager", def.Metadata.DisplayName)
	assert.Equal(t, "v1.17.2", def.Metadata.Version)
	assert.Equal(t, "Fundament", def.Metadata.Author)
	assert.Equal(t, "Apache-2.0", def.Metadata.License)
	assert.Equal(t, "shield-check", def.Metadata.Icon)
	assert.Equal(t, "https://cert-manager.io", def.Metadata.URLs.Homepage)
	assert.NotEmpty(t, def.Permissions.Capabilities)
	assert.NotEmpty(t, def.Permissions.RBAC)
	assert.Len(t, def.Menu.Organization, 1)
	assert.Len(t, def.Menu.Project, 2)
}

func TestNewCertManagerPlugin(t *testing.T) {
	def := pluginsdk.PluginDefinition{
		Metadata: pluginsdk.PluginMetadata{
			Name:    "cert-manager",
			Version: "v1.17.2",
		},
	}

	plugin := NewCertManagerPlugin(def)
	assert.Equal(t, def, plugin.Definition())
}

func TestDefinitionFields(t *testing.T) {
	def, err := pluginsdk.LoadDefinition("definition.yaml")
	require.NoError(t, err)

	plugin := NewCertManagerPlugin(def)
	got := plugin.Definition()

	assert.Equal(t, "cert-manager", got.Metadata.Name)
	assert.Equal(t, "Cert Manager", got.Metadata.DisplayName)
	assert.Equal(t, "v1.17.2", got.Metadata.Version)
}

func TestPluginImplementsInterfaces(t *testing.T) {
	def := pluginsdk.PluginDefinition{}
	plugin := NewCertManagerPlugin(def)

	// Verify all expected interfaces are implemented.
	var _ pluginsdk.Plugin = plugin
	var _ pluginsdk.Installer = plugin
	var _ pluginsdk.Reconciler = plugin
	var _ pluginsdk.ConsoleProvider = plugin
}
