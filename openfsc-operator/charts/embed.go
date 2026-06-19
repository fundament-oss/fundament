// Package charts embeds the vendored OpenFSC charts so the operator installs
// them with no network fetch: the digilab umbrella
// (gitlab.com/digilab.overheid.nl/platform/helm-charts/open-fsc) and the
// open-fsc-inway / open-fsc-outway charts
// (oci://registry-1.docker.io/federatedserviceconnectivity).
package charts

import "embed"

// The all: prefix keeps files that go:embed would otherwise skip
// (templates/_helpers.tpl starts with an underscore).
//
//go:embed open-fsc-2.4.0.tgz all:open-fsc-inway all:open-fsc-outway
var FS embed.FS

// Version is the vendored chart version, shared by the umbrella and the
// gateway charts. The reconciler upgrades releases whose deployed chart
// version differs, so bumping the vendored charts rolls out on its own.
const Version = "2.4.0"

const (
	UmbrellaArchive = "open-fsc-" + Version + ".tgz"
	InwayDir        = "open-fsc-inway"
	OutwayDir       = "open-fsc-outway"
)
