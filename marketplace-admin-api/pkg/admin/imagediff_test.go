package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
)

func pkg(name, version string) analyzedPackage {
	return analyzedPackage{
		Purl: "pkg:deb/debian/" + name + "@" + version + "?distro=debian-12",
		Name: name, Version: version, Type: "deb",
	}
}

type packageRow struct {
	name, version, baseVersion string
	change                     adminv1.Change
}

func packageRows(diffs []packageDiff) []packageRow {
	rows := make([]packageRow, 0, len(diffs))
	for _, d := range diffs {
		rows = append(rows, packageRow{d.pkg.Name, d.pkg.Version, d.baseVersion, d.change})
	}
	return rows
}

func TestPackageKeyDropsVersionAndQualifiers(t *testing.T) {
	for purl, key := range map[string]string{
		"pkg:deb/debian/openssl@3.0.13?distro=debian-12.5": "pkg:deb/debian/openssl",
		"pkg:npm/%40angular/core@18.0.0":                   "pkg:npm/%40angular/core",
		"pkg:golang/golang.org/x/net@v0.30.0#http2":        "pkg:golang/golang.org/x/net",
		"pkg:generic/thing":                                "pkg:generic/thing",
	} {
		assert.Equal(t, key, packageKey(purl), purl)
	}
}

func TestDiffPackages(t *testing.T) {
	tests := []struct {
		name     string
		current  []analyzedPackage
		base     []analyzedPackage
		compared bool
		want     []packageRow
	}{
		{
			name:     "not compared",
			current:  []analyzedPackage{pkg("openssl", "3.0.13")},
			compared: false,
			want:     []packageRow{{"openssl", "3.0.13", "", adminv1.Change_CHANGE_UNSPECIFIED}},
		},
		{
			name:     "unchanged, changed, added and removed",
			current:  []analyzedPackage{pkg("openssl", "3.0.14"), pkg("zlib", "1.3"), pkg("curl", "8.5")},
			base:     []analyzedPackage{pkg("openssl", "3.0.13"), pkg("zlib", "1.3"), pkg("bash", "5.2")},
			compared: true,
			want: []packageRow{
				{"bash", "", "5.2", adminv1.Change_CHANGE_REMOVED},
				{"curl", "8.5", "", adminv1.Change_CHANGE_ADDED},
				{"openssl", "3.0.14", "3.0.13", adminv1.Change_CHANGE_CHANGED},
				{"zlib", "1.3", "", adminv1.Change_CHANGE_UNCHANGED},
			},
		},
		{
			// A module vendored into two binaries, one of which bumped it.
			name:     "duplicates match equal versions first",
			current:  []analyzedPackage{pkg("x-net", "0.31"), pkg("x-net", "0.30")},
			base:     []analyzedPackage{pkg("x-net", "0.30"), pkg("x-net", "0.30")},
			compared: true,
			want: []packageRow{
				{"x-net", "0.30", "", adminv1.Change_CHANGE_UNCHANGED},
				{"x-net", "0.31", "0.30", adminv1.Change_CHANGE_CHANGED},
			},
		},
		{
			name:     "a third copy is added",
			current:  []analyzedPackage{pkg("x-net", "0.30"), pkg("x-net", "0.30")},
			base:     []analyzedPackage{pkg("x-net", "0.30")},
			compared: true,
			want: []packageRow{
				{"x-net", "0.30", "", adminv1.Change_CHANGE_UNCHANGED},
				{"x-net", "0.30", "", adminv1.Change_CHANGE_ADDED},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := packageRows(diffPackages(tt.current, tt.base, tt.compared))
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestDiffFiles(t *testing.T) {
	current := []analyzedFile{
		{Path: "/etc/config", Mode: 0o644, SHA256: "new"},
		{Path: "/usr/bin/plugin", Mode: 0o755, SHA256: "same"},
		{Path: "/usr/bin/run", Mode: 0o777, Link: "plugin"},
		{Path: "/opt/added", Mode: 0o644, SHA256: "a"},
	}
	base := []analyzedFile{
		{Path: "/etc/config", Mode: 0o644, SHA256: "old"},
		{Path: "/usr/bin/plugin", Mode: 0o755, SHA256: "same"},
		{Path: "/usr/bin/run", Mode: 0o777, Link: "plugin-v1"},
		{Path: "/etc/removed", Mode: 0o644, SHA256: "r"},
	}

	diffs := diffFiles(current, base, true)
	got := map[string]adminv1.Change{}
	order := make([]string, 0, len(diffs))
	for _, d := range diffs {
		got[d.file.Path] = d.change
		order = append(order, d.file.Path)
	}

	assert.Equal(t, map[string]adminv1.Change{
		"/etc/config":     adminv1.Change_CHANGE_CHANGED,
		"/usr/bin/plugin": adminv1.Change_CHANGE_UNCHANGED,
		"/usr/bin/run":    adminv1.Change_CHANGE_CHANGED,
		"/opt/added":      adminv1.Change_CHANGE_ADDED,
		"/etc/removed":    adminv1.Change_CHANGE_REMOVED,
	}, got)
	assert.Equal(t, []string{"/etc/config", "/etc/removed", "/opt/added", "/usr/bin/plugin", "/usr/bin/run"}, order)
}

func TestDiffFilesModeChangeCounts(t *testing.T) {
	diffs := diffFiles(
		[]analyzedFile{{Path: "/bin/tool", Mode: 0o4755, SHA256: "x"}},
		[]analyzedFile{{Path: "/bin/tool", Mode: 0o755, SHA256: "x"}},
		true,
	)
	assert.Equal(t, adminv1.Change_CHANGE_CHANGED, diffs[0].change)
}

func TestKeep(t *testing.T) {
	assert.False(t, keep(adminv1.Change_CHANGE_UNCHANGED, true, ""))
	assert.True(t, keep(adminv1.Change_CHANGE_UNSPECIFIED, true, ""), "nothing to hide without a comparison")
	assert.True(t, keep(adminv1.Change_CHANGE_ADDED, true, "ssl", "OpenSSL"))
	assert.False(t, keep(adminv1.Change_CHANGE_ADDED, false, "zlib", "openssl", "pkg:deb/debian/openssl@3"))
}

func TestParsePageToken(t *testing.T) {
	offset, err := parsePageToken("", 10)
	assert.NoError(t, err)
	assert.Equal(t, 0, offset)

	offset, err = parsePageToken("5", 10)
	assert.NoError(t, err)
	assert.Equal(t, 5, offset)

	for _, bad := range []string{"-1", "11", "abc"} {
		_, err := parsePageToken(bad, 10)
		assert.Error(t, err, bad)
	}
}
