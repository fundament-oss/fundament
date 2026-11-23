package outbox

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The README draws the outbox row's states. The drawing is generated from the
// table, so it cannot promise an edge the worker refuses.
func TestReadmeDiagramMatchesTheMachine(t *testing.T) {
	t.Parallel()

	readme, err := os.ReadFile("../../README.md")
	require.NoError(t, err)

	updated, err := ReplaceReadmeDiagram(string(readme))
	require.NoError(t, err)
	assert.Equal(t, string(readme), updated,
		"the README's outbox diagram is out of date: run go generate ./cluster-worker/pkg/outbox/")
}
