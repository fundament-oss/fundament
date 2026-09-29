package kube

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A client naming the proxy's impersonation headers in Connection must not get
// them removed: the request would run as the proxy's own ServiceAccount.
func TestRewrite_ConnectionCannotStripProxyImpersonation(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), SATokenContextKey{}, "proxy-sa-token")
	ctx = WithImpersonation(ctx, Impersonation{
		User:   "system:serviceaccount:fundament-system:fundament-u1",
		Groups: []string{"fundament:namespace-listers"},
	})
	h := http.Header{}
	h.Set("Connection", "Impersonate-User, impersonate-group, Authorization")

	seen := forwardThrough(t, ctx, h)

	assert.Equal(t, "system:serviceaccount:fundament-system:fundament-u1", seen.Get("Impersonate-User"))
	assert.Equal(t, []string{"fundament:namespace-listers"}, seen.Values("Impersonate-Group"))
	assert.Equal(t, "Bearer proxy-sa-token", seen.Get("Authorization"))
}

func TestRewrite_ConnectionCannotStripUserToken(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), SATokenContextKey{}, "user-sa-token")
	h := http.Header{}
	h.Set("Connection", "Authorization")

	seen := forwardThrough(t, ctx, h)

	assert.Equal(t, "Bearer user-sa-token", seen.Get("Authorization"))
}

func TestSandboxRewrite_ConnectionCannotStripPluginToken(t *testing.T) {
	t.Parallel()
	var seen http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := buildSandboxReverseProxy(target, http.DefaultTransport, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx := context.WithValue(context.Background(), SATokenContextKey{}, "plugin-sa-token")
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/pods", http.NoBody)
	req.Header.Set("Connection", "Authorization")
	proxy.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, seen, "upstream not reached")
	assert.Equal(t, "Bearer plugin-sa-token", seen.Get("Authorization"))
}

// The apiserver's audit log keeps the chain of source IPs, as it did with the
// Director: inbound forwarding headers are kept and the client IP is appended.
func TestRewrite_KeepsForwardingHeaders(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), SATokenContextKey{}, "user-sa-token")
	h := http.Header{}
	h.Set("X-Forwarded-For", "203.0.113.7")
	h.Set("X-Forwarded-Proto", "https")
	h.Set("X-Forwarded-Host", "k8s-api.example")

	seen := forwardThrough(t, ctx, h)

	// httptest.NewRequest's RemoteAddr is 192.0.2.1.
	assert.Equal(t, "203.0.113.7, 192.0.2.1", seen.Get("X-Forwarded-For"))
	assert.Equal(t, "https", seen.Get("X-Forwarded-Proto"))
	assert.Equal(t, "k8s-api.example", seen.Get("X-Forwarded-Host"))
}
