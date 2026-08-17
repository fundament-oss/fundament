package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// ParseDefinition requires a pinned image. The source definition.yaml is an
// image-free template; publish injects the real digest.
const fakeDigest = "ghcr.io/fundament-oss/fundament/ceph-rook-plugin@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

// injectFakeImage sets spec.image so ParseDefinition can validate the template.
func injectFakeImage(src []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal definition yaml: %w", err)
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
	for i := 0; i+1 < len(spec.Content); i += 2 {
		if spec.Content[i].Value == "image" {
			spec.Content[i+1].Value = fakeDigest
			out, err := yaml.Marshal(&doc)
			if err != nil {
				return nil, fmt.Errorf("marshal definition yaml: %w", err)
			}
			return out, nil
		}
	}
	spec.Content = append(spec.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "image"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fakeDigest},
	)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("marshal definition yaml: %w", err)
	}
	return out, nil
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
			for slot, file := range map[string]string{
				"list":   mapping.List,
				"detail": mapping.Detail,
				"create": mapping.Create,
			} {
				if file == "" {
					continue
				}
				path := "console/" + file
				_, statErr := os.Stat(path)
				assert.NoError(t, statErr, "customComponents[%s].%s file must exist: %s", kind, slot, path)
			}
		}
	})

	t.Run("customComponents/disk-has-detail", func(t *testing.T) {
		t.Parallel()
		mapping, ok := def.Spec.CustomComponents["Disk"]
		require.True(t, ok, "Disk must declare custom components")
		assert.NotEmpty(t, mapping.Detail, "Disk must have a detail page")
	})

	// A page on disk that nothing references is unreachable.
	t.Run("customComponents/every-page-is-referenced", func(t *testing.T) {
		t.Parallel()
		referenced := make(map[string]struct{})
		for _, mapping := range def.Spec.CustomComponents {
			for _, file := range []string{mapping.List, mapping.Detail, mapping.Create} {
				if file != "" {
					referenced[file] = struct{}{}
				}
			}
		}
		entries, err := os.ReadDir("console")
		require.NoError(t, err)
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".html") {
				continue
			}
			assert.Contains(t, referenced, entry.Name(),
				"console/%s is not referenced from customComponents, so the host can never route to it", entry.Name())
		}
	})
}

// Run only registers /console/ for a ConsoleProvider; without it the embedded
// files are never served and the iframe 404s.
func TestPluginServesConsoleAssets(t *testing.T) {
	t.Parallel()
	plugin, err := NewPlugin()
	require.NoError(t, err)

	provider, ok := any(plugin).(pluginruntime.ConsoleProvider)
	require.True(t, ok, "Plugin must implement pluginruntime.ConsoleProvider")

	f, err := provider.ConsoleAssets().Open("/storagepools-list.html")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
