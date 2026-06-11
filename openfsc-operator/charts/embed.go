// Package charts embeds the vendored OpenFSC charts the operator installs at
// runtime with no network fetch: the digilab umbrella
// (gitlab.com/digilab.overheid.nl/platform/helm-charts/open-fsc, version
// 1.43.0) with the Fundament values override, and the open-fsc-inway /
// open-fsc-outway charts installed one release per Inway/Outway CR.
package charts

import "embed"

// FS holds the vendored charts. The all: prefix keeps files that go:embed
// would otherwise skip (templates/_helpers.tpl starts with an underscore).
//
//go:embed open-fsc-1.43.0.tgz values-fundament.yaml all:open-fsc-inway all:open-fsc-outway
var FS embed.FS

// Paths within FS.
const (
	// UmbrellaArchive is the vendored OpenFSC umbrella chart (.tgz).
	UmbrellaArchive = "open-fsc-1.43.0.tgz"
	// ValuesFundament is the Fundament override layered on the umbrella's values.
	ValuesFundament = "values-fundament.yaml"
	// InwayDir is the unpacked open-fsc-inway chart directory.
	InwayDir = "open-fsc-inway"
	// OutwayDir is the unpacked open-fsc-outway chart directory.
	OutwayDir = "open-fsc-outway"
)
