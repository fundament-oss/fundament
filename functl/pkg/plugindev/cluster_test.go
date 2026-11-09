package plugindev

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func crdVersion(name string, served, dev bool) map[string]any {
	spec := map[string]any{}
	if dev {
		spec["dev"] = map[string]any{"type": "object"}
	}
	return map[string]any{
		"name":   name,
		"served": served,
		"schema": map[string]any{"openAPIV3Schema": map[string]any{
			"properties": map[string]any{"spec": map[string]any{"properties": spec}},
		}},
	}
}

func TestServedVersionHasDev(t *testing.T) {
	assert.True(t, servedVersionHasDev([]any{crdVersion("v1", true, true)}))
	assert.False(t, servedVersionHasDev([]any{crdVersion("v1", true, false)}))
	// spec.dev on another version doesn't count.
	assert.False(t, servedVersionHasDev([]any{crdVersion("v2", true, true), crdVersion("v1", true, false)}))
	assert.False(t, servedVersionHasDev([]any{crdVersion("v1", false, true)}))
}
