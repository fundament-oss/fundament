package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// fakeDigest is a valid-looking digest reference used to satisfy
// ParseDefinition's requirement for a pinned image. The source definition.yaml
// is an image-free template; publish time injects the real digest.
const fakeDigest = "ghcr.io/fundament-oss/fundament/ceph-rook-plugin@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

// injectFakeImage sets spec.image on the raw YAML bytes so that
// ParseDefinition (which requires a digest reference) can validate the
// source definition template.
func injectFakeImage(src []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	root := doc.Content[0]
	var spec *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "spec" {
			spec = root.Content[i+1]
			break
		}
	}
	if spec == nil || spec.Kind != yaml.MappingNode {
		return nil, nil
	}
	// Set spec.image.
	for i := 0; i+1 < len(spec.Content); i += 2 {
		if spec.Content[i].Value == "image" {
			spec.Content[i+1].Value = fakeDigest
			out, err := yaml.Marshal(&doc)
			return out, err
		}
	}
	spec.Content = append(spec.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "image"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fakeDigest},
	)
	return yaml.Marshal(&doc)
}

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

	// Inject a fake digest so ParseDefinition can validate the source template.
	withImage, err := injectFakeImage(src)
	require.NoError(t, err)

	def, err := pluginruntime.ParseDefinition(withImage)
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

	t.Run("customComponents/html-files-exist", func(t *testing.T) {
		t.Parallel()
		for kind, mapping := range def.Spec.CustomComponents {
			if mapping.List != "" {
				path := "console/" + mapping.List
				_, statErr := os.Stat(path)
				assert.NoError(t, statErr, "customComponents[%s].list file must exist: %s", kind, path)
			}
			if mapping.Detail != "" {
				path := "console/" + mapping.Detail
				_, statErr := os.Stat(path)
				assert.NoError(t, statErr, "customComponents[%s].detail file must exist: %s", kind, path)
			}
		}
	})
}
