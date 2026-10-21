package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestInjectImage(t *testing.T) {
	src := []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.17.2
spec:
  permissions:
    rbac:
      - apiGroups: [cert-manager.io]
        resources: [certificates]
        verbs: [get]
`)
	out, err := injectImage(src, "localhost:5112/cert-manager-plugin@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "IfNotPresent")
	require.NoError(t, err)
	def, err := pluginruntime.ParseDefinition(out) // strict: proves image is present
	require.NoError(t, err)
	assert.Equal(t, "localhost:5112/cert-manager-plugin@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", def.Spec.Image)
	assert.Equal(t, "IfNotPresent", def.Spec.ImagePullPolicy)
	// original RBAC survives
	require.Len(t, def.Spec.Permissions.RBAC, 1)
}

func TestPluginPublishRefusesDevAndSubmit(t *testing.T) {
	cmd := &PluginPublishCmd{Dev: true, Submit: true}
	err := cmd.Run(&Context{})
	assert.ErrorContains(t, err, "--dev and --submit cannot be combined")
}

func TestInjectDevVersion(t *testing.T) {
	src := []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
  version: v1.16.1
spec:
  image: localhost:5112/cert-manager-plugin@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  permissions:
    rbac:
      - apiGroups: [cert-manager.io]
        resources: [certificates]
        verbs: [get]
`)
	out, err := injectDevVersion(src)
	require.NoError(t, err)
	def, err := pluginruntime.ParseDefinition(out)
	require.NoError(t, err)
	assert.Equal(t, "v1.16.1-dev", def.Metadata.Version)
	require.Len(t, def.Spec.Permissions.RBAC, 1) // rest of the yaml survives

	// idempotent: already -dev is left alone
	again, err := injectDevVersion(out)
	require.NoError(t, err)
	def, err = pluginruntime.ParseDefinition(again)
	require.NoError(t, err)
	assert.Equal(t, "v1.16.1-dev", def.Metadata.Version)

	// errors when metadata.version is absent
	noVersion := []byte(`apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: cert-manager
spec:
  image: localhost:5112/cert-manager-plugin@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  permissions:
    rbac: []
`)
	_, err = injectDevVersion(noVersion)
	assert.Error(t, err)
}
