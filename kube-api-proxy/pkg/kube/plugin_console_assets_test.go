package kube

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func policy() ConsoleAssetPolicy {
	return ConsoleAssetPolicy{
		AssetOrigin:    "https://k8s-api.example",
		ConsoleOrigins: []string{"https://console.example", "http://localhost:4200"},
	}
}

func TestNormalizeOrigins(t *testing.T) {
	// CONSOLE_ORIGINS is comma-separated and hand-written, so spacing and a stray
	// trailing slash are realistic.
	require.Equal(t,
		[]string{"https://console.example", "http://localhost:4200"},
		NormalizeOrigins([]string{" https://console.example/ ", "", "http://localhost:4200"}),
	)
}

func TestConsoleAssetPolicyAllowsHost(t *testing.T) {
	p := policy()

	require.True(t, p.AllowsHost("https://console.example"))
	require.True(t, p.AllowsHost("http://localhost:4200"))
	// The unframed dev preview loads its assets relatively and sends no host.
	require.True(t, p.AllowsHost(""))
	// The whole point: a link crafted with someone else's origin is not served.
	require.False(t, p.AllowsHost("https://evil.example"))
	// A prefix of an allowed origin is a different origin.
	require.False(t, p.AllowsHost("https://console.example.evil.test"))

	// With no Console origins configured (bare local dev) the check stands down.
	require.True(t, ConsoleAssetPolicy{}.AllowsHost("https://evil.example"))
}

func TestConsoleAssetPolicySetHeaders(t *testing.T) {
	h := http.Header{}
	// Simulate the CORS middleware having already reflected an origin with credentials.
	h.Set("Access-Control-Allow-Origin", "https://console.example")
	h.Set("Access-Control-Allow-Credentials", "true")

	policy().SetHeaders(h)

	require.Equal(t, "*", h.Get("Access-Control-Allow-Origin"))
	require.Empty(t, h.Get("Access-Control-Allow-Credentials"))

	csp := h.Get("Content-Security-Policy")
	// script-src is the header's reason for existing: only the asset origin and the
	// Console may supply scripts, whatever the asset's ?host= says.
	require.Contains(t, csp,
		"script-src 'self' https://console.example http://localhost:4200 https://k8s-api.example")
	require.Contains(t, csp, "default-src 'none'")
	require.Contains(t, csp, "connect-src 'none'")
	require.Contains(t, csp, "frame-ancestors https://console.example http://localhost:4200")
	// The design system's fonts are inlined as data: URIs.
	require.Contains(t, csp, "data:")
}

// Without configured Console origins there is nothing to scope a CSP to, so the
// policy stands down rather than serve a header that would block the plugin's own
// bundles.
func TestConsoleAssetPolicySetHeadersWithoutOriginsSetsNoCSP(t *testing.T) {
	h := http.Header{}

	ConsoleAssetPolicy{}.SetHeaders(h)

	require.Equal(t, "*", h.Get("Access-Control-Allow-Origin"))
	require.Empty(t, h.Get("Content-Security-Policy"))
}
