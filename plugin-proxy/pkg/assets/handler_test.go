package assets

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

const testPluginName = "cert-manager"

var testClusterID = uuid.MustParse("019b4000-2000-7000-8000-000000000001")

type stubResolver struct{ clusterID uuid.UUID }

func (s stubResolver) ClusterFor(_ context.Context, _, _ string) (uuid.UUID, error) {
	return s.clusterID, nil
}

type stubFetcher struct{}

func (stubFetcher) Fetch(_ context.Context, clusterID uuid.UUID, pluginName, assetPath string) ([]byte, string, error) {
	return []byte("body-" + clusterID.String() + pluginName + assetPath), guessContentType(assetPath), nil
}

func newTestHandler() http.Handler {
	return NewHandler(stubResolver{clusterID: testClusterID}, stubFetcher{}, &CSPConfig{
		ConnectSrc:     []string{"https://kube-api-proxy.test", "https://plugin-proxy.test"},
		FormAction:     []string{"https://kube-api-proxy.test", "https://plugin-proxy.test"},
		FrameAncestors: []string{"https://console.test"},
	}, discardLogger())
}

func TestHandler_ServesAssetsWithImmutableCache(t *testing.T) {
	h := newTestHandler()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/plugins/"+testPluginName+"/v1/console/index.html", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code, "body = %s", w.Body.String())
	assert.Contains(t, w.Header().Get("Cache-Control"), "immutable")
	assert.True(t, strings.HasPrefix(w.Header().Get("Content-Type"), "text/html"),
		"Content-Type = %q", w.Header().Get("Content-Type"))
}

func TestHandler_SetsStrictCSP(t *testing.T) {
	h := newTestHandler()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/plugins/"+testPluginName+"/v1/console/index.html", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"connect-src https://kube-api-proxy.test https://plugin-proxy.test",
		"form-action https://kube-api-proxy.test https://plugin-proxy.test",
		"frame-ancestors https://console.test",
		"base-uri 'none'",
		"object-src 'none'",
	} {
		assert.Contains(t, csp, want)
	}
	// FUN-17 forbids 'unsafe-inline' on the plugin path.
	assert.NotContains(t, csp, "unsafe-inline")
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}

func TestHandler_RejectsBadPath(t *testing.T) {
	h := newTestHandler()
	for _, p := range []string{
		"/plugins/cert-manager/v1/console/",              // empty file
		"/plugins/cert-manager/v1/console/../etc/passwd", // traversal
		"/plugins//v1/console/index.html",                // empty name
		"/plugins/cert-manager//console/index.html",      // empty version
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, http.NoBody)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		assert.GreaterOrEqual(t, w.Code, 300, "expected non-2xx for %q, got %d", p, w.Code)
	}
}

func TestHandler_OnlyGETandHEAD(t *testing.T) {
	h := newTestHandler()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/plugins/x/v/console/i.html", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
