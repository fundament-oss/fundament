package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func parseDefinition(t *testing.T) pluginruntime.PluginDefinition {
	t.Helper()
	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err, "definition.yaml must exist")
	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err, "definition.yaml must parse without error")
	return def
}

func TestDefinition(t *testing.T) {
	t.Parallel()
	def := parseDefinition(t)

	// The seed catalog row and the Console's installation name depend on it.
	assert.Equal(t, "cloudnativepg", def.Metadata.Name)

	t.Run("allowedResources", func(t *testing.T) {
		t.Parallel()
		verbs := make(map[string][]string)
		for _, r := range def.Spec.AllowedResources {
			verbs[r.Resource] = r.Verbs
		}
		assert.ElementsMatch(t, []string{"list", "get", "create"}, verbs["clusters"])
		assert.ElementsMatch(t, []string{"list"}, verbs["storageclasses"])
	})

	// The plugin SA caps the console pages: no rule may let them change or
	// delete a database.
	t.Run("rbac/clusters-cannot-be-modified", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, rule := range def.Spec.Permissions.RBAC {
			if !slices.Contains(rule.Resources, "clusters") {
				continue
			}
			found = true
			for _, verb := range []string{"update", "patch", "delete", "deletecollection", "*"} {
				assert.NotContains(t, rule.Verbs, verb, "the clusters rule must not grant %s", verb)
			}
		}
		assert.True(t, found, "permissions.rbac must grant access to clusters")
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

	// The generated detail view offers Delete; only a custom one keeps it out.
	t.Run("customComponents/cluster-has-detail", func(t *testing.T) {
		t.Parallel()
		mapping, ok := def.Spec.CustomComponents["Cluster"]
		require.True(t, ok, "Cluster must declare custom components")
		assert.NotEmpty(t, mapping.Detail, "Cluster must have a custom detail page")
		assert.NotEmpty(t, mapping.Create, "Cluster must have a create page")
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

// A database cannot be changed or deleted from the console. The plugin's rbac
// already refuses it; this keeps the pages from offering it.
func TestConsoleOffersNoModification(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("console")
	require.NoError(t, err)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		src, err := os.ReadFile("console/" + entry.Name())
		require.NoError(t, err)
		assert.NotContains(t, string(src), "k8s.delete", "console/%s must not delete", entry.Name())
		assert.NotContains(t, string(src), "k8s.patch", "console/%s must not patch", entry.Name())
	}
}

// Run only registers /console/ for a ConsoleProvider; without it the embedded
// files are never served and the iframe 404s.
func TestPluginServesConsoleAssets(t *testing.T) {
	t.Parallel()
	plugin, err := newPlugin()
	require.NoError(t, err)

	provider, ok := any(plugin).(pluginruntime.ConsoleProvider)
	require.True(t, ok, "Plugin must implement pluginruntime.ConsoleProvider")

	f, err := provider.ConsoleAssets().Open("/clusters-create.html")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
