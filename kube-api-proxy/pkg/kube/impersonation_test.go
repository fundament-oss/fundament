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

// forwardThrough sends req through buildReverseProxy to a recording upstream
// and returns the headers the upstream saw.
func forwardThrough(t *testing.T, ctx context.Context, header http.Header) http.Header {
	t.Helper()
	var seen http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	proxy := buildReverseProxy(target, http.DefaultTransport, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/namespaces", http.NoBody)
	req.Header = header
	proxy.ServeHTTP(httptest.NewRecorder(), req)
	require.NotNil(t, seen, "upstream not reached")
	return seen
}

func clientImpersonationHeaders() http.Header {
	h := http.Header{}
	h.Set("Impersonate-User", "system:admin")
	h.Add("Impersonate-Group", "system:masters")
	h.Set("Impersonate-Uid", "1")
	h.Set("Impersonate-Extra-Scopes", "all")
	h["impersonate-user"] = []string{"lowercase-sneak"} // non-canonical key
	return h
}

// With the caller's own token, kubectl --as reaches the apiserver unchanged:
// the caller's RBAC decides whether they may impersonate.
func TestRewrite_PassesClientImpersonationWithUserToken(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), SATokenContextKey{}, "user-sa-token")
	h := http.Header{}
	h.Set("Impersonate-User", "system:serviceaccount:shop:app")
	h.Add("Impersonate-Group", "shop-devs")

	seen := forwardThrough(t, ctx, h)

	assert.Equal(t, "system:serviceaccount:shop:app", seen.Get("Impersonate-User"))
	assert.Equal(t, []string{"shop-devs"}, seen.Values("Impersonate-Group"))
	assert.Equal(t, "Bearer user-sa-token", seen.Get("Authorization"))
}

// With the proxy's own token, client impersonation must never get through:
// the proxy's ServiceAccount may impersonate identities the caller may not.

func TestRewrite_AppliesProxyImpersonationOnly(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), SATokenContextKey{}, "proxy-sa-token")
	ctx = WithImpersonation(ctx, Impersonation{
		User:   "system:serviceaccount:fundament-system:fundament-u1",
		Groups: []string{"fundament:namespace-listers"},
	})

	seen := forwardThrough(t, ctx, clientImpersonationHeaders())

	assert.Equal(t, "system:serviceaccount:fundament-system:fundament-u1", seen.Get("Impersonate-User"))
	assert.Equal(t, []string{"fundament:namespace-listers"}, seen.Values("Impersonate-Group"))
	assert.Empty(t, seen.Get("Impersonate-Uid"))
	assert.Empty(t, seen.Get("Impersonate-Extra-Scopes"))
	assert.Equal(t, "Bearer proxy-sa-token", seen.Get("Authorization"))
}

func TestHasClientImpersonation(t *testing.T) {
	t.Parallel()
	assert.True(t, HasClientImpersonation(clientImpersonationHeaders()))
	assert.True(t, HasClientImpersonation(http.Header{"impersonate-uid": {"1"}}), "non-canonical key")
	assert.False(t, HasClientImpersonation(http.Header{"Authorization": {"Bearer x"}}))
	assert.False(t, HasClientImpersonation(http.Header{}))
}
