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

// A request with a retry gets a new answer instead of the cluster's 401 for a
// token whose ServiceAccount was deleted or created again.
func TestReverseProxy_UnauthorizedCallsRetry(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := buildReverseProxy(target, http.DefaultTransport, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var calls []string
	ctx := WithTokenRejected(context.WithValue(t.Context(), SATokenContextKey{}, "sa-token"), func() { calls = append(calls, "rejected") })
	ctx = WithTokenRetry(ctx, func(w http.ResponseWriter) {
		calls = append(calls, "retry")
		w.WriteHeader(http.StatusForbidden)
	})
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/namespaces", http.NoBody))
	assert.Equal(t, http.StatusForbidden, rec.Code, "the retry answers, not the 401")
	assert.Equal(t, []string{"rejected", "retry"}, calls, "the token is dropped before the retry fetches a new one")
}

// A 401 from a workload behind pods, services or nodes /proxy says nothing
// about the token: it is passed on, and the token stays cached.
func TestReverseProxy_WorkloadUnauthorizedKeepsToken(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := buildReverseProxy(target, http.DefaultTransport, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var calls []string
	ctx := WithTokenRejected(context.WithValue(t.Context(), SATokenContextKey{}, "sa-token"), func() { calls = append(calls, "rejected") })
	ctx = WithTokenRetry(ctx, func(http.ResponseWriter) { calls = append(calls, "retry") })
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/namespaces/apps/pods/web/proxy/login", http.NoBody))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, calls)
}

func TestIsProxySubresource(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]bool{
		"/api/v1/namespaces/apps/pods/web/proxy/login":      true,
		"/api/v1/namespaces/apps/services/web:80/proxy/":    true,
		"/api/v1/nodes/node-1/proxy/metrics":                true,
		"/api/v1/namespaces/apps/configmaps":                false,
		"/api/v1/namespaces/apps/pods/web/log":              false,
		"/apis/metrics.k8s.io/v1beta1/namespaces/apps/pods": false,
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
		assert.Equal(t, want, isProxySubresource(r), path)
	}
}

// TestWithTokenRejected_KeepsOuterCallback pins that a request that swaps in
// another token still drops the one it was given first.
func TestWithTokenRejected_KeepsOuterCallback(t *testing.T) {
	t.Parallel()
	var dropped []string
	ctx := WithTokenRejected(t.Context(), func() { dropped = append(dropped, "user") })
	ctx = WithTokenRejected(ctx, func() { dropped = append(dropped, "proxy") })

	tokenRejected(ctx)
	assert.Equal(t, []string{"proxy", "user"}, dropped)
}
