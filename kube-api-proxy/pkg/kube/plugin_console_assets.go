package kube

import (
	"net/http"
	"strings"
)

// ConsoleAssetPolicy is the cross-origin policy for plugin console assets — the
// static HTML/JS/CSS that make up a plugin's iframe UI.
//
// These responses are unusual in two ways, and the two combine badly if left
// unguarded:
//
//   - They are served *without authentication* (see IsPluginConsoleAssetPath): the
//     sandboxed iframe that loads them has an opaque origin and cannot send
//     credentials.
//   - The asset HTML bootstraps itself from an origin named in its own `?host=`
//     query param — it injects `<script src="${host}/plugin-ui/...">` to pull the
//     plugin SDK and the design system from the Console (see the plugin console-ui's
//     shared.ts).
//
// Without a check, anyone could hand a victim a link to a console asset with a
// `?host=` of their choosing and get script execution on this origin — the same
// origin whose cookie authorizes `/clusters/{id}/...`. Two things prevent that:
//
//   - AllowsHost rejects a request whose `?host=` is not a Console origin, so the
//     bad URL never renders.
//   - SetHeaders serves every asset under a CSP whose script-src names only the
//     asset origin and the Console origins, so even a `?host=` that somehow got
//     through cannot load a foreign bundle.
//
// A zero policy (no origins configured — a bare local dev setup) stands down rather
// than break the iframe; kube-api-proxy logs a warning at startup. Every deployed
// environment configures it via the Helm chart.
type ConsoleAssetPolicy struct {
	// AssetOrigin is the proxy's own public origin, the one console assets are
	// served from. It is named explicitly rather than relying on CSP's 'self'
	// because the iframe is sandboxed without allow-same-origin: its document has an
	// opaque origin, and what 'self' resolves to there is not something the plugin's
	// own bundles should depend on.
	AssetOrigin string
	// ConsoleOrigins are the origins the Console is served from — the only ones a
	// console asset may be bootstrapped from.
	ConsoleOrigins []string
}

// NormalizeOrigin cleans one configured origin: trims whitespace and any trailing
// slash (an origin has no path). Returns "" for a blank entry.
func NormalizeOrigin(origin string) string {
	return strings.TrimSuffix(strings.TrimSpace(origin), "/")
}

// NormalizeOrigins cleans a configured origin list, dropping empty entries.
// The env vars behind these are comma-separated and hand-written, so "a, b/" is a
// realistic input.
func NormalizeOrigins(origins []string) []string {
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		if o := NormalizeOrigin(origin); o != "" {
			normalized = append(normalized, o)
		}
	}
	return normalized
}

// Configured reports whether the policy has everything it needs to protect console
// assets: the Console origins to allow-list a `?host=` against, and the proxy's own
// origin to admit the plugin's bundles from. Both halves are required — a CSP naming
// only the Console would block the plugin's own scripts.
//
// An unconfigured policy stands down (no `?host=` check, no CSP) rather than break
// the iframe, which suits a bare local dev setup. proxy.New rejects it outright in
// "real" mode, where standing down would mean shipping the hole this policy exists
// to close.
func (p ConsoleAssetPolicy) Configured() bool {
	return len(p.ConsoleOrigins) > 0 && p.AssetOrigin != ""
}

// AllowsHost reports whether a console asset may be served with the given `?host=`
// origin. An empty host is the unframed dev preview, which loads its assets
// relatively; an unconfigured policy stands down.
func (p ConsoleAssetPolicy) AllowsHost(host string) bool {
	if host == "" || !p.Configured() {
		return true
	}
	host = NormalizeOrigin(host)
	for _, origin := range p.ConsoleOrigins {
		if strings.EqualFold(origin, host) {
			return true
		}
	}
	return false
}

// SetHeaders stamps the policy onto a console asset response. Shared by mock mode
// and the real-mode proxy.
//
// Access-Control-Allow-Origin is "*" because the sandboxed iframe that loads these
// has an opaque origin (Origin: null) that cannot be allow-listed; credentials are
// dropped to match. The CSP is what keeps that openness from being exploitable.
func (p ConsoleAssetPolicy) SetHeaders(h http.Header) {
	h.Set("Access-Control-Allow-Origin", "*")
	h.Del("Access-Control-Allow-Credentials")
	if csp := p.contentSecurityPolicy(); csp != "" {
		h.Set("Content-Security-Policy", csp)
	}
}

// contentSecurityPolicy builds the CSP for a console asset, or "" when the policy is
// not fully configured (see Configured).
//
// A plugin UI only ever loads its own bundles (from the asset origin) and the shared
// /plugin-ui/ bundles from the Console, so everything else is 'none':
//
//   - script-src names exactly those two sources. This is the header's reason for
//     existing. There is deliberately no 'self': the iframe is sandboxed without
//     allow-same-origin, so its document has an opaque origin and 'self' matches
//     nothing — AssetOrigin is what actually admits the plugin's own bundles, which
//     is why an unset one suppresses the header entirely rather than emitting a CSP
//     that would block them.
//   - style-src needs 'unsafe-inline' for the style="" attributes the plugin markup
//     uses; that grants no script execution.
//   - font-src/img-src allow data: — the design system's fonts are inlined as data:
//     URIs precisely so the opaque-origin iframe needs no cross-origin font fetch.
//   - connect-src 'none': plugin views reach the cluster through postMessage to the
//     host, never over the network themselves.
func (p ConsoleAssetPolicy) contentSecurityPolicy() string {
	if !p.Configured() {
		return ""
	}
	sources := append([]string{p.AssetOrigin}, p.ConsoleOrigins...)
	origins := strings.Join(sources, " ")
	return strings.Join([]string{
		"default-src 'none'",
		"script-src " + origins,
		"style-src " + origins + " 'unsafe-inline'",
		"font-src " + origins + " data:",
		"img-src " + origins + " data:",
		"connect-src 'none'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors " + strings.Join(p.ConsoleOrigins, " "),
	}, "; ")
}
