package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// Gateways are Namespaced, so they belong under menu.project only.
func TestDefinition_MenuScope(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err)

	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err)

	assert.Empty(t, def.Spec.Menu.Organization)
	crds := make([]string, 0, len(def.Spec.Menu.Project))
	for _, e := range def.Spec.Menu.Project {
		crds = append(crds, e.CRD)
	}
	assert.Contains(t, crds, "gateways.gateway.networking.k8s.io")
}

// The console loads a menu entry's schema only when spec.crds lists its CRD.
func TestDefinition_MenuCRDsListed(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err)

	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err)

	for _, e := range def.Spec.Menu.Project {
		assert.Contains(t, def.Spec.CRDs, e.CRD)
		assert.NotEmpty(t, e.Label, e.CRD)
	}
}
