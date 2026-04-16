package pluginruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDef(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "definition.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadDefinition_MissingMetadataName(t *testing.T) {
	path := writeDef(t, `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  displayName: Example
spec: {}
`)

	_, err := LoadDefinition(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata.name")
}

func TestLoadDefinition_UnsupportedAPIVersion(t *testing.T) {
	path := writeDef(t, `apiVersion: other/v1
kind: PluginDefinition
metadata:
  name: example
spec: {}
`)

	_, err := LoadDefinition(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported apiVersion")
}

func TestLoadDefinition_UnsupportedKind(t *testing.T) {
	path := writeDef(t, `apiVersion: fundament.io/v1
kind: Other
metadata:
  name: example
spec: {}
`)

	_, err := LoadDefinition(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported kind")
}
