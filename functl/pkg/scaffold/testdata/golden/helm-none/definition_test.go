package main

import (
	"os"
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
