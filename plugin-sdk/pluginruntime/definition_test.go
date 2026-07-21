package pluginruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
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

// resourceNames declared in a plugin's definition.yaml must survive the
// YAML→proto mapping in GetDefinition — otherwise the controller can never
// scope the materialised ClusterRole to named objects.
func TestGetDefinition_ThreadsResourceNames(t *testing.T) {
	path := writeDef(t, `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: example
spec:
  permissions:
    rbac:
      - apiGroups: [""]
        resources: ["secrets"]
        verbs: ["get"]
        resourceNames: ["example-ca"]
`)
	def, err := LoadDefinition(path)
	require.NoError(t, err)

	h := NewMetadataHandler(
		func() PluginStatus { return PluginStatus{} },
		func() PluginDefinition { return def },
		func(context.Context) error { return nil },
	)
	resp, err := h.GetDefinition(context.Background(), connect.NewRequest(&pb.GetDefinitionRequest{}))
	require.NoError(t, err)

	rbac := resp.Msg.GetPermissions().GetRbac()
	require.Len(t, rbac, 1)
	assert.Equal(t, []string{"example-ca"}, rbac[0].GetResourceNames())
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
