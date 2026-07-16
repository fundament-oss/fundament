package console

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
)

// Option configures NewFileSystem.
type Option func(*options)

type options struct {
	requireHTML bool
}

// RequireHTML makes NewFileSystem panic when the embedded subtree holds no HTML file.
//
// A plugin whose console is a built UI wants this. Such a console/ directory is a
// build artifact: it is gitignored and seeded with only a .gitkeep, so
// `go:embed console/*` still compiles on a fresh checkout even when the UI has never
// been built. The result is a binary that serves an empty console — a blank plugin
// iframe at runtime, with nothing in the logs. Failing at startup turns that into an
// obvious error at the point the mistake was made.
//
// It is opt-in rather than the runtime's default because "a console is at least one
// HTML file" is a fact about how a particular plugin builds its UI, not about what
// this runtime can serve: a third-party plugin is free to embed a console that is
// nothing but JS.
func RequireHTML() Option {
	return func(o *options) { o.requireHTML = true }
}

// NewFileSystem creates an http.FileSystem from an fs.FS, rooted at the given
// subdirectory. This is intended for use with embed.FS in ConsoleProvider
// implementations. Plugins that build their console UI should pass RequireHTML so an
// unbuilt UI fails at startup rather than serving a blank iframe.
func NewFileSystem(fsys fs.FS, root string, opts ...Option) http.FileSystem {
	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}

	sub, err := fs.Sub(fsys, root)
	if err != nil {
		panic(fmt.Sprintf("console: invalid root %q: %v", root, err))
	}
	if cfg.requireHTML && !containsHTML(sub) {
		panic(fmt.Sprintf(
			"console: no HTML files embedded under %q — the console UI was not built "+
				"before `go build`. Run the console-ui build (see the plugin's Dockerfile "+
				"console-ui stage, or `bun run build` in its console-ui/ directory).", root))
	}
	return http.FS(sub)
}

// containsHTML reports whether fsys holds at least one .html file at any depth.
func containsHTML(fsys fs.FS) bool {
	found := false
	// The error is ignored: a walk failure means we found nothing, which is exactly
	// what `found` already reports.
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && path.Ext(p) == ".html" {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}
