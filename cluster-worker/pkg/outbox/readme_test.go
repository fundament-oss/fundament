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

// An end marker above the block does not close it: a stray marker must not make
// the generator fail, nor rewrite everything up to it.
func TestReplaceReadmeDiagramIgnoresAnEarlierEndMarker(t *testing.T) {
	t.Parallel()

	readme := readmeEnd + "\n\n" + readmeDiagram() + "\n\ntail\n"
	updated, err := ReplaceReadmeDiagram(readme)
	require.NoError(t, err)
	assert.Equal(t, readme, updated)
}

func TestReplaceReadmeDiagramNeedsBothMarkers(t *testing.T) {
	t.Parallel()

	_, err := ReplaceReadmeDiagram("no markers here")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start marker")

	_, err = ReplaceReadmeDiagram(readmeBegin + "\n\nno end\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "end marker")
}
