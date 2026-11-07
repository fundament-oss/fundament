// Package catalogseed reads the marketplace listings in
// db/seed/0101-appstore-catalog.sql, so a plugin's tests can check that the
// versions its listing names are the ones its code installs.
// It finds the seed from its own source path, so it does not work under
// go test -trimpath.
package catalogseed

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// InstalledVersions returns the "## Installed versions" list items of the
// listing named name.
func InstalledVersions(t testing.TB, name string) []string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	seed, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "db", "seed", "0101-appstore-catalog.sql"))
	require.NoError(t, err)

	rowStart := regexp.MustCompile(`\n {4}\('[0-9a-f-]+', '[0-9a-f-]+', '([a-z0-9-]+)', `)
	var rows [][]byte
	starts := rowStart.FindAllSubmatchIndex(seed, -1)
	for i, m := range starts {
		if string(seed[m[2]:m[3]]) != name {
			continue
		}
		end := len(seed)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		rows = append(rows, seed[m[0]:end])
	}
	require.Len(t, rows, 1, "listings named %s", name)
	row := rows[0]

	section := regexp.MustCompile(`\n## Installed versions\n\n((?:- [^\n]*\n)+)`).FindSubmatch(row)
	require.NotNil(t, section, "no Installed versions section in the %s listing", name)
	matches := regexp.MustCompile(`(?m)^- (.*)$`).FindAllSubmatch(section[1], -1)
	items := make([]string, 0, len(matches))
	for _, m := range matches {
		items = append(items, string(m[1]))
	}
	return items
}
