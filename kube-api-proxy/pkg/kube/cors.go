package kube

import "net/http"

// SetPluginConsoleAssetCORS applies the CORS headers for a plugin console
// asset response. These assets are public and loaded as ES modules from a
// sandboxed iframe with an opaque (`null`) origin, so they must allow any
// origin. `Access-Control-Allow-Origin: *` MUST NOT be paired with
// `Access-Control-Allow-Credentials: true` (browsers reject the combination
// for credentialed requests), so any credentials header set by an outer CORS
// middleware is cleared here.
//
// Shared by the mock handler and the real-mode reverse-proxy writer so the two
// paths cannot drift.
func SetPluginConsoleAssetCORS(h http.Header) {
	h.Set("Access-Control-Allow-Origin", "*")
	h.Del("Access-Control-Allow-Credentials")
}
