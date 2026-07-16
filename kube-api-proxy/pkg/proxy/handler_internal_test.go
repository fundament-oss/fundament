package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

const consoleAssetPath = "api/v1/namespaces/plugin-openfsc/services/http:plugin-openfsc:8080/proxy/console/assets/app.js"

var testConsoleAssets = kube.ConsoleAssetPolicy{
	AssetOrigin:    "https://k8s-api.example",
	ConsoleOrigins: []string{"https://console.example"},
}

// assetBackend simulates the proxied plugin pod in real mode: it serves a static
// asset and sets no CORS or CSP headers of its own.
type assetBackend struct{}

func (assetBackend) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("export const x = 1;"))
}

func newAssetServer() http.Handler {
	s := &Server{kubeHandler: assetBackend{}, consoleAssets: testConsoleAssets}
	mux := http.NewServeMux()
	mux.Handle("/clusters/{clusterID}/{path...}", http.HandlerFunc(s.handleClusterProxy))
	return mux
}

func newAssetRequest(query string) *http.Request {
	url := "/clusters/" + uuid.NewString() + "/" + consoleAssetPath + query
	return httptest.NewRequest(http.MethodGet, url, nil)
}

// A plugin console asset served through the cluster proxy must carry a public
// CORS policy so the sandboxed, opaque-origin iframe can load it — even when the
// CORS middleware reflected a (different) allow-listed origin with credentials,
// and even though the backend pod sets no CORS headers.
func TestHandleClusterProxy_ForcesPublicCORSOnConsoleAssets(t *testing.T) {
	req := newAssetRequest("?host=https://console.example")
	rec := httptest.NewRecorder()
	// Simulate the CORS middleware having reflected an allow-listed origin with
	// credentials; the override must replace both for the public asset.
	rec.Header().Set("Access-Control-Allow-Origin", "https://console.example")
	rec.Header().Set("Access-Control-Allow-Credentials", "true")

	newAssetServer().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "export const x = 1;", rec.Body.String())
	require.Equal(t, "text/javascript", rec.Header().Get("Content-Type"))
	require.Len(t, rec.Header().Values("Access-Control-Allow-Origin"), 1, "exactly one ACAO header")
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}

// The asset HTML injects <script src="${host}/plugin-ui/..."> from its own `?host=`
// param, and these paths are served unauthenticated — so the CSP that confines those
// scripts to the Console must be on every console asset response.
func TestHandleClusterProxy_SetsCSPOnConsoleAssets(t *testing.T) {
	rec := httptest.NewRecorder()

	newAssetServer().ServeHTTP(rec, newAssetRequest("?host=https://console.example"))

	csp := rec.Header().Get("Content-Security-Policy")
	require.Contains(t, csp, "script-src https://k8s-api.example https://console.example")
	require.Contains(t, csp, "default-src 'none'")
}

// A hand-crafted link with someone else's `?host=` would otherwise turn a public,
// unauthenticated asset into script execution on this origin.
func TestHandleClusterProxy_RejectsForeignHostOrigin(t *testing.T) {
	rec := httptest.NewRecorder()

	newAssetServer().ServeHTTP(rec, newAssetRequest("?host=https://evil.example"))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, rec.Body.String(), "export const x", "the asset must not be served at all")
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

// The override must also apply when the backend writes the body without an
// explicit WriteHeader (implicit 200).
func TestPluginAssetHeaderWriter_ImplicitWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &pluginAssetHeaderWriter{ResponseWriter: rec, policy: testConsoleAssets}

	_, err := w.Write([]byte("body"))
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	require.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
}

// Flushing an uncommitted response sends the header block as it stands, so the
// policy has to be applied by then — otherwise it escapes on a flushed response.
func TestPluginAssetHeaderWriter_FlushAppliesHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &pluginAssetHeaderWriter{ResponseWriter: rec, policy: testConsoleAssets}

	w.Flush()

	require.True(t, rec.Flushed)
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	require.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
}

// httputil.ReverseProxy reaches the writer through http.ResponseController, which
// walks Unwrap. Without it the proxy's flush and deadline calls hit the wrapper and
// fail with http.ErrNotSupported instead of the real writer.
func TestPluginAssetHeaderWriter_ResponseControllerReachesWrappedWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &pluginAssetHeaderWriter{ResponseWriter: rec, policy: testConsoleAssets}

	require.Same(t, rec, w.Unwrap())
	require.NoError(t, http.NewResponseController(w).Flush())
	require.True(t, rec.Flushed)
}
