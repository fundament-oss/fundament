package scaffold

// DefaultSDKVersion is the fundament version that newly scaffolded plugins pin
// to get the plugin SDK (github.com/fundament-oss/fundament/plugin-sdk/...).
//
// It is a baked-in constant rather than something resolved at run time on
// purpose: `functl plugin create` must work with no network and no credentials,
// and querying the module proxy for the latest version would break both.
//
// It is a pseudo-version because the repository carries no semver tag yet. A
// pseudo-version needs no tag -- the module proxy synthesizes one for any commit
// it can reach -- so a scaffolded project tidies and builds today. It must point
// at a commit that actually contains the SDK API the templates call, so bump it
// together with any template change that uses a newly added SDK symbol.
//
// Replace it with a plain vX.Y.Z once the first fundament tag is pushed: a real
// tag sorts above every pseudo-version and reads as a version rather than a
// timestamp.
const DefaultSDKVersion = "v0.0.0-20260902143725-454dc6e5d341"
