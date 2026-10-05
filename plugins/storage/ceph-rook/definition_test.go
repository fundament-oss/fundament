package main

import (
	"os"
	"strings"
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

	// patch, not update: the edit forms merge-patch. No delete: the console
	// offers none, since the guards it would need (bound volumes, bound
	// bucket claims) can only be meaningful server-side.
	t.Run("allowedResources", func(t *testing.T) {
		t.Parallel()
		for resource, verbs := range map[string][]string{
			"disks":          {"list", "get"},
			"diskpools":      {"list", "get", "create", "patch"},
			"blockstorages":  {"list", "get", "create", "patch"},
			"filestorages":   {"list", "get", "create", "patch"},
			"objectstorages": {"list", "get", "create", "patch"},
		} {
			var found *pluginruntime.AllowedResource
			for i := range def.Spec.AllowedResources {
				if def.Spec.AllowedResources[i].Resource == resource {
					found = &def.Spec.AllowedResources[i]
					break
				}
			}
			require.NotNil(t, found, "allowedResources must contain %s", resource)
			assert.ElementsMatch(t, verbs, found.Verbs, resource)
		}
	})

	// The three consumer kinds share one status vocabulary, expressed in the
	// YAML as an anchor/alias; this proves the parser resolves it into every
	// kind, so the badge tables cannot drift apart.
	t.Run("uiHints/consumer-kinds-share-status-mapping", func(t *testing.T) {
		t.Parallel()
		block, ok := def.Spec.UIHints["blockstorages.ceph.fundament.io"]
		require.True(t, ok, "blockstorages must declare uiHints")
		require.Contains(t, block.StatusMapping.Values, "Ready")
		require.Contains(t, block.StatusMapping.Values, "Provisioning")
		require.Contains(t, block.StatusMapping.Values, "Degraded")
		for _, crd := range []string{"filestorages.ceph.fundament.io", "objectstorages.ceph.fundament.io"} {
			hint, ok := def.Spec.UIHints[crd]
			require.True(t, ok, "%s must declare uiHints", crd)
			assert.Equal(t, block.StatusMapping, hint.StatusMapping, crd)
		}
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

// The console deliberately offers no way to delete a DiskPool. The guard
// against deleting one whose StorageClass still has volumes bound ran in the
// browser, while the API accepted the same delete from kubectl, Flux or any
// other client — a guard in one client that reads as a platform guarantee. The
// button belongs back only once that check is server-side, so this test fails
// if it returns before then.
func TestConsoleOffersNoDiskPoolDelete(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"console/diskpools-detail.html",
		"console/diskpools-detail.js",
		"console/diskpools-list.js",
		// The blockstorages-*/filestorages-* files are thin wrappers; the page
		// code they share lives in consumer-pages.js.
		"console/consumer-pages.js",
		"console/blockstorages-detail.js",
		"console/blockstorages-list.js",
		"console/filestorages-detail.js",
		"console/filestorages-list.js",
		"console/objectstorages-detail.js",
		"console/objectstorages-list.js",
	} {
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.NotContains(t, string(src), "k8s.delete", "%s must not delete a DiskPool", path)
		assert.NotContains(t, string(src), "delete-btn", "%s must not offer a delete button", path)
	}
}

// Run only registers /console/ for a ConsoleProvider; without it the embedded
// files are never served and the iframe 404s.
func TestPluginServesConsoleAssets(t *testing.T) {
	t.Parallel()
	plugin, err := NewPlugin()
	require.NoError(t, err)

	provider, ok := any(plugin).(pluginruntime.ConsoleProvider)
	require.True(t, ok, "Plugin must implement pluginruntime.ConsoleProvider")

	f, err := provider.ConsoleAssets().Open("/diskpools-list.html")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
