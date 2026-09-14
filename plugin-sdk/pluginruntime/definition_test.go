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
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  imagePullPolicy: IfNotPresent
  permissions:
    rbac:
      - apiGroups: [cert-manager.io]
        resources: [certificates]
        verbs: [get, list]
`)
	def, err := ParseDefinition(data)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", def.Spec.Image)
	assert.Equal(t, "IfNotPresent", def.Spec.ImagePullPolicy)
	require.Len(t, def.Spec.Permissions.RBAC, 1)
	assert.Equal(t, []string{"cert-manager.io"}, def.Spec.Permissions.RBAC[0].APIGroups)
}

func TestParseDefinition_RejectsWrongKind(t *testing.T) {
	_, err := ParseDefinition([]byte("apiVersion: fundament.io/v1\nkind: Nope\n"))
	require.Error(t, err)
}

func TestParseDefinition_RejectsMutableTagImage(t *testing.T) {
	// A published definition must pin an immutable digest, not a mutable tag.
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller:v1.17.2
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest reference")
}

func TestParseDefinition_RejectsWrongLengthDigest(t *testing.T) {
	// A sha256 digest is exactly 64 hex chars; a truncated one must be rejected.
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeef
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest reference")
}

func TestParseDefinition_RejectsInvalidImagePullPolicy(t *testing.T) {
	_, err := ParseDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  image: quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  imagePullPolicy: always
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "imagePullPolicy")
}

func TestHashManifest_Stable(t *testing.T) {
	manifest := []byte("hello")
	assert.Equal(t, HashManifest(manifest), HashManifest(manifest))
	assert.True(t, len(HashManifest(manifest)) == len("sha256:")+64)
	assert.NotEqual(t, HashManifest([]byte("a")), HashManifest([]byte("b")))
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

func TestParseSourceDefinition_AcceptsMissingImage(t *testing.T) {
	// The authored source template has no image; publish injects the digest.
	def, err := ParseSourceDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  permissions:
    rbac: []
`))
	require.NoError(t, err)
	assert.Equal(t, "cert-manager", def.Metadata.Name)
	assert.Empty(t, def.Spec.Image)
}

func TestParseSourceDefinition_RejectsMutableTag(t *testing.T) {
	// An image that IS present must still be digest-pinned, so a hand-written
	// mutable tag fails here rather than at publish time.
	_, err := ParseSourceDefinition([]byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
spec:
  image: quay.io/jetstack/cert-manager-controller:v1.17.2
  permissions:
    rbac: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest reference")
}

func TestParseSourceDefinition_StillValidatesEnvelope(t *testing.T) {
	for name, manifest := range map[string]string{
		"apiVersion": `apiVersion: fundament.io/v2
kind: PluginDefinition
metadata:
  name: cert-manager
spec:
  permissions:
    rbac: []
`,
		"kind": `apiVersion: fundament.io/v1
kind: Plugin
metadata:
  name: cert-manager
spec:
  permissions:
    rbac: []
`,
		"metadata.name": `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  version: v1.0.0
spec:
  permissions:
    rbac: []
`,
		"unknown field": `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
spec:
  permissionz:
    rbac: []
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseSourceDefinition([]byte(manifest))
			require.Error(t, err)
		})
	}
}
