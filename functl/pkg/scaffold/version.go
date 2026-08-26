package scaffold

// DefaultSDKVersion is the plugin-sdk release that newly scaffolded plugins pin.
//
// It is a baked-in constant rather than something resolved at run time on
// purpose: `functl plugin create` must work with no network and no credentials,
// and querying the module proxy for the latest version would break both.
//
// Bump this whenever a new plugin-sdk/vX.Y.Z tag is pushed (see
// plugin-sdk/README.md).
const DefaultSDKVersion = "v0.1.0"
