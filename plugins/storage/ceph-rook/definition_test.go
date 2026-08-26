package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestPluginImplementsInterface(t *testing.T) {
	t.Parallel()
	plugin, err := NewPlugin()
	require.NoError(t, err)

	var _ pluginruntime.Plugin = plugin

	assert.NotNil(t, plugin)
}

func TestDefinition(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err, "definition.yaml must exist")

	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err, "definition.yaml must parse without error")

	assert.Equal(t, "ceph-rook", def.Metadata.Name)

	t.Run("allowedResources/storagepools", func(t *testing.T) {
		t.Parallel()
		var found *pluginruntime.AllowedResource
		for i := range def.Spec.AllowedResources {
			if def.Spec.AllowedResources[i].Resource == "storagepools" {
				found = &def.Spec.AllowedResources[i]
				break
			}
		}
		require.NotNil(t, found, "allowedResources must contain storagepools")
		assert.ElementsMatch(t, []string{"list", "get", "create", "update", "delete"}, found.Verbs)
	})

	t.Run("allowedResources/disks", func(t *testing.T) {
		t.Parallel()
		var found *pluginruntime.AllowedResource
		for i := range def.Spec.AllowedResources {
			if def.Spec.AllowedResources[i].Resource == "disks" {
				found = &def.Spec.AllowedResources[i]
				break
			}
		}
		require.NotNil(t, found, "allowedResources must contain disks")
		assert.ElementsMatch(t, []string{"list", "get"}, found.Verbs)
	})

}
