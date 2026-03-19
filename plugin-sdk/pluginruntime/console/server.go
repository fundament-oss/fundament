package console

import (
	"fmt"
	"io/fs"
	"net/http"
)

// NewFileSystem creates an http.FileSystem from an fs.FS, rooted at the given
// subdirectory. This is intended for use with embed.FS in ConsoleProvider
// implementations.
func NewFileSystem(fsys fs.FS, root string) http.FileSystem {
	sub, err := fs.Sub(fsys, root)
	if err != nil {
		panic(fmt.Sprintf("console: invalid root %q: %v", root, err))
	}
	return http.FS(sub)
}
