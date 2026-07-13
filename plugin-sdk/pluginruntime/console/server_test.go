package console

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestNewFileSystem(t *testing.T) {
	fsys := fstest.MapFS{
		"console/index.html":       {Data: []byte("<html></html>")},
		"console/assets/app-x1.js": {Data: []byte("export const x = 1;")},
	}

	fileSystem := NewFileSystem(fsys, "console")

	f, err := fileSystem.Open("/index.html")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// A plugin's console/ is gitignored and only .gitkeep-seeded, so `go:embed
// console/*` compiles even when the UI was never built. Serving that silently
// would give a blank iframe with nothing in the logs, so it must fail loudly.
func TestNewFileSystemPanicsWhenConsoleUnbuilt(t *testing.T) {
	fsys := fstest.MapFS{"console/.gitkeep": {Data: []byte{}}}

	require.PanicsWithValue(t,
		"console: no HTML files embedded under \"console\" — the console UI was not built "+
			"before `go build`. Run the console-ui build (see the plugin's Dockerfile "+
			"console-ui stage, or `bun run build` in its console-ui/ directory).",
		func() { NewFileSystem(fsys, "console") },
	)
}

func TestNewFileSystemPanicsOnInvalidRoot(t *testing.T) {
	require.Panics(t, func() { NewFileSystem(fstest.MapFS{}, "../escape") })
}
