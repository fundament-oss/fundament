package admin

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
)

// The shapes marketplace-analysis-worker stores in plugin_image_analyses.
type analyzedPackage struct {
	Purl     string   `json:"purl"`
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Type     string   `json:"type"`
	Licenses []string `json:"licenses"`
}

type analyzedFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mode   int64  `json:"mode"`
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

type packageDiff struct {
	pkg         analyzedPackage
	change      adminv1.Change
	baseVersion string
}

type fileDiff struct {
	file   analyzedFile
	change adminv1.Change
}

// packageKey identifies a package across versions: its purl without the
// version, qualifiers or subpath. pkg:deb/debian/openssl@3.0.13?distro=x and
// pkg:deb/debian/openssl@3.0.14?distro=y are the same package at two versions;
// the qualifiers go too because a distro qualifier changes with every base
// image bump.
func packageKey(purl string) string {
	if i := strings.IndexAny(purl, "@?#"); i >= 0 {
		return purl[:i]
	}
	return purl
}

// diffPackages compares two package lists. One package can appear more than
// once (a Go module vendored into two binaries), so entries under one key are
// matched as a multiset: equal versions pair up as UNCHANGED, the leftovers
// pair up as CHANGED, and what remains on either side is ADDED or REMOVED.
// Without a base every entry's change is UNSPECIFIED.
func diffPackages(current, base []analyzedPackage, compared bool) []packageDiff {
	if !compared {
		diffs := make([]packageDiff, 0, len(current))
		for _, pkg := range current {
			diffs = append(diffs, packageDiff{pkg: pkg, change: adminv1.Change_CHANGE_UNSPECIFIED})
		}
		sortPackageDiffs(diffs)
		return diffs
	}

	baseByKey := make(map[string][]analyzedPackage)
	for _, pkg := range base {
		key := packageKey(pkg.Purl)
		baseByKey[key] = append(baseByKey[key], pkg)
	}
	currentByKey := make(map[string][]analyzedPackage)
	for _, pkg := range current {
		key := packageKey(pkg.Purl)
		currentByKey[key] = append(currentByKey[key], pkg)
	}

	var diffs []packageDiff
	for key, now := range currentByKey {
		before := baseByKey[key]
		var unmatched []analyzedPackage
		for _, pkg := range now {
			if i := indexOfVersion(before, pkg.Version); i >= 0 {
				diffs = append(diffs, packageDiff{pkg: pkg, change: adminv1.Change_CHANGE_UNCHANGED})
				before = append(before[:i:i], before[i+1:]...)
				continue
			}
			unmatched = append(unmatched, pkg)
		}
		for _, pkg := range unmatched {
			if len(before) > 0 {
				diffs = append(diffs, packageDiff{pkg: pkg, change: adminv1.Change_CHANGE_CHANGED, baseVersion: before[0].Version})
				before = before[1:]
				continue
			}
			diffs = append(diffs, packageDiff{pkg: pkg, change: adminv1.Change_CHANGE_ADDED})
		}
		baseByKey[key] = before
	}
	for _, remaining := range baseByKey {
		for _, pkg := range remaining {
			removed := pkg
			removed.Version = ""
			diffs = append(diffs, packageDiff{pkg: removed, change: adminv1.Change_CHANGE_REMOVED, baseVersion: pkg.Version})
		}
	}

	sortPackageDiffs(diffs)
	return diffs
}

func indexOfVersion(pkgs []analyzedPackage, version string) int {
	for i, pkg := range pkgs {
		if pkg.Version == version {
			return i
		}
	}
	return -1
}

func sortPackageDiffs(diffs []packageDiff) {
	sort.SliceStable(diffs, func(i, j int) bool {
		if diffs[i].pkg.Name != diffs[j].pkg.Name {
			return diffs[i].pkg.Name < diffs[j].pkg.Name
		}
		if diffs[i].pkg.Purl != diffs[j].pkg.Purl {
			return diffs[i].pkg.Purl < diffs[j].pkg.Purl
		}
		return diffs[i].change < diffs[j].change
	})
}

// diffFiles compares two file lists by path. A file is CHANGED when its
// contents, mode or link target differ.
func diffFiles(current, base []analyzedFile, compared bool) []fileDiff {
	diffs := make([]fileDiff, 0, len(current))
	if !compared {
		for _, file := range current {
			diffs = append(diffs, fileDiff{file: file, change: adminv1.Change_CHANGE_UNSPECIFIED})
		}
		sortFileDiffs(diffs)
		return diffs
	}

	baseByPath := make(map[string]analyzedFile, len(base))
	for _, file := range base {
		baseByPath[file.Path] = file
	}
	for _, file := range current {
		before, ok := baseByPath[file.Path]
		delete(baseByPath, file.Path)
		change := adminv1.Change_CHANGE_ADDED
		if ok {
			change = adminv1.Change_CHANGE_UNCHANGED
			if before.SHA256 != file.SHA256 || before.Mode != file.Mode || before.Link != file.Link {
				change = adminv1.Change_CHANGE_CHANGED
			}
		}
		diffs = append(diffs, fileDiff{file: file, change: change})
	}
	for _, file := range baseByPath {
		diffs = append(diffs, fileDiff{file: file, change: adminv1.Change_CHANGE_REMOVED})
	}

	sortFileDiffs(diffs)
	return diffs
}

func sortFileDiffs(diffs []fileDiff) {
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].file.Path < diffs[j].file.Path })
}

func summarize(changes []adminv1.Change) *adminv1.ChangeSummary {
	var unchanged, added, removed, changed int32
	for _, change := range changes {
		switch change {
		case adminv1.Change_CHANGE_UNCHANGED:
			unchanged++
		case adminv1.Change_CHANGE_ADDED:
			added++
		case adminv1.Change_CHANGE_REMOVED:
			removed++
		case adminv1.Change_CHANGE_CHANGED:
			changed++
		case adminv1.Change_CHANGE_UNSPECIFIED:
			// Not compared: nothing to count.
		default:
			panic("unhandled Change: " + change.String())
		}
	}
	return adminv1.ChangeSummary_builder{
		Unchanged: unchanged, Added: added, Removed: removed, Changed: changed,
	}.Build()
}

// keep reports whether an entry passes changes_only and query. query is
// already lower-cased.
func keep(change adminv1.Change, changesOnly bool, query string, fields ...string) bool {
	if changesOnly && change == adminv1.Change_CHANGE_UNCHANGED {
		return false
	}
	if query == "" {
		return true
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

// The page token is the offset into the filtered list. It is opaque to the
// client, but nothing secret: a client can only page through what it may
// read anyway.
func parsePageToken(token string, total int) (int, error) {
	if token == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset < 0 || offset > total {
		return 0, errors.New("invalid files_page_token")
	}
	return offset, nil
}
