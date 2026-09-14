package main

import (
	"io/fs"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// definition.yaml is the authored, image-free source manifest, so it is parsed
// with ParseSourceDefinition. Publishing injects the pushed image digest and
// validates the result with the stricter ParseDefinition.
func TestDefinitionParses(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err, "definition.yaml must exist")

	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err, "definition.yaml must parse without error")

	assert.Equal(t, "demo", def.Metadata.Name)
}

// TestCustomComponentsExist guards the one coupling nothing else checks: every
// file named in spec.customComponents has to be embedded under console/, or the
// console renders a blank iframe at run time with nothing in the logs.
func TestCustomComponentsExist(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err)
	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err)

	entries, err := fs.ReadDir(consoleFiles, "console")
	require.NoError(t, err)
	if len(entries) <= 1 {
		t.Skip("console/ is not built; run `cd console-ui && bun run build`")
	}

	for kind, components := range def.Spec.CustomComponents {
		for label, file := range map[string]string{
			"list":   components.List,
			"detail": components.Detail,
			"create": components.Create,
		} {
			if file == "" {
				continue
			}
			_, err := fs.Stat(consoleFiles, path.Join("console", file))
			require.NoError(t, err, "customComponents.%s.%s references %q, which is not embedded under console/", kind, label, file)
		}
	}
}
