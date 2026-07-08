package pluginruntime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDefinition_ImageAndRBAC(t *testing.T) {
	data := []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeef
  imagePullPolicy: IfNotPresent
  permissions:
    rbac:
      - apiGroups: [cert-manager.io]
        resources: [certificates]
        verbs: [get, list]
`)
	def, err := ParseDefinition(data)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/jetstack/cert-manager-controller@sha256:deadbeef", def.Spec.Image)
	assert.Equal(t, "IfNotPresent", def.Spec.ImagePullPolicy)
	require.Len(t, def.Spec.Permissions.RBAC, 1)
	assert.Equal(t, []string{"cert-manager.io"}, def.Spec.Permissions.RBAC[0].APIGroups)
}

func TestParseDefinition_RejectsWrongKind(t *testing.T) {
	_, err := ParseDefinition([]byte("apiVersion: fundament.io/v1\nkind: Nope\n"))
	require.Error(t, err)
}

func TestParseDefinition_RejectsMissingImage(t *testing.T) {
	// The image-free source template is NOT a valid PluginDefinition.
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.image")
}
