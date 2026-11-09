package scaffold

// DefaultSDKVersion is the fundament version that newly scaffolded plugins pin
// to get the plugin SDK (github.com/fundament-oss/fundament/plugin-sdk/...).
//
// It is baked in rather than resolved at run time on purpose: `functl plugin
// create` must work with no network and no credentials, and querying the module
// proxy for the latest version would break both.
//
// Release builds overwrite it with the pseudo-version of the master commit they
// are built from (-ldflags -X, see .github/workflows/release-functl.yml), so a
// released functl pins exactly the SDK its templates were written against. The
// value below is the fallback for every other build. It must point at a commit
// on master, or the module proxy cannot resolve it, and that commit must contain
// the SDK API the templates call: bump it together with any template change
// that uses a newly added SDK symbol.
//
// Replace the pseudo-version with a plain vX.Y.Z once the first fundament tag is
// pushed; the release workflow then has to stamp that tag too.
var DefaultSDKVersion = "v0.0.0-20261105173205-d80f3ad60d43"
