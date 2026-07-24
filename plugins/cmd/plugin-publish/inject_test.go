package main

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
