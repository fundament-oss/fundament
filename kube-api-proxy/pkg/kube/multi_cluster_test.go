package kube

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMultiClusterProxy() *MultiClusterProxy {
	return NewMultiClusterProxy(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// serveThrough sends one request for clusterID through m and returns the
// response code and whether its SA token was rejected.
func serveThrough(t *testing.T, m *MultiClusterProxy, clusterID string) (code int, rejected bool) {
	t.Helper()
	ctx := context.WithValue(t.Context(), ClusterIDContextKey{}, clusterID)
	ctx = context.WithValue(ctx, SATokenContextKey{}, "sa-token")
	ctx = WithTokenRejected(ctx, func() { rejected = true })
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/namespaces", http.NoBody)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	return rec.Code, rejected
}

// TestMultiClusterProxy_UntrustedCertificateDropsCachedProxy pins that a
// cluster created again under the same address, with a new certificate
// authority, gets fresh credentials and a fresh SA token on the next request
// instead of failing until the cached ones expire.
func TestMultiClusterProxy_UntrustedCertificateDropsCachedProxy(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	m := newTestMultiClusterProxy()
	// The default transport does not trust the test server's certificate,
	// like the cached transport of the cluster's previous incarnation.
	m.storeProxy("c1", target, http.DefaultTransport, time.Now().Add(time.Hour))

	code, rejected := serveThrough(t, m, "c1")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.True(t, rejected, "the new cluster signs SA tokens with a new key")
	_, cached := m.proxies.Load("c1")
	assert.False(t, cached)
}

func TestMultiClusterProxy_UnreachableClusterKeepsCachedProxy(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	upstream.Close()

	m := newTestMultiClusterProxy()
	m.storeProxy("c1", target, http.DefaultTransport, time.Now().Add(time.Hour))

	code, rejected := serveThrough(t, m, "c1")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.False(t, rejected)
	_, cached := m.proxies.Load("c1")
	assert.True(t, cached, "a cluster that does not answer says nothing about its credentials")
}

func TestReverseProxy_UnauthorizedReportsRejectedToken(t *testing.T) {
	t.Parallel()
	code := http.StatusUnauthorized
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := buildReverseProxy(target, http.DefaultTransport, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rejected := 0
	ctx := WithTokenRejected(context.WithValue(t.Context(), SATokenContextKey{}, "sa-token"), func() { rejected++ })
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/namespaces", http.NoBody))
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "the 401 is passed on")
	assert.Equal(t, 1, rejected)

	code = http.StatusForbidden
	proxy.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/namespaces", http.NoBody))
	assert.Equal(t, 1, rejected, "a denied request does not reject the token")
}
