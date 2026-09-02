package scaffold

// DefaultSDKVersion is the fundament release that newly scaffolded plugins pin
// to get the plugin SDK (github.com/fundament-oss/fundament/plugin-sdk/...).
//
// It is a baked-in constant rather than something resolved at run time on
// purpose: `functl plugin create` must work with no network and no credentials,
// and querying the module proxy for the latest version would break both.
//
// Bump this whenever a new fundament vX.Y.Z tag is pushed.
const DefaultSDKVersion = "v0.1.0"
