package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// assetBackend simulates the proxied plugin pod in real mode: it serves a static
// asset and sets no CORS headers of its own.
type assetBackend struct{}

func (assetBackend) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("export const x = 1;"))
}

// A plugin console asset served through the cluster proxy must carry a public
// CORS policy so the sandboxed, opaque-origin iframe can load it — even when the
// CORS middleware reflected a (different) allow-listed origin with credentials,
// and even though the backend pod sets no CORS headers.
func TestHandleClusterProxy_ForcesPublicCORSOnConsoleAssets(t *testing.T) {
	s := &Server{kubeHandler: assetBackend{}}
	mux := http.NewServeMux()
	mux.Handle("/clusters/{clusterID}/{path...}", http.HandlerFunc(s.handleClusterProxy))

	path := "api/v1/namespaces/plugin-openfsc/services/http:plugin-openfsc:8080/proxy/console/assets/app.js"
	req := httptest.NewRequest(http.MethodGet, "/clusters/"+uuid.NewString()+"/"+path, nil)
	rec := httptest.NewRecorder()
	// Simulate the CORS middleware having reflected an allow-listed origin with
	// credentials; the override must replace both for the public asset.
	rec.Header().Set("Access-Control-Allow-Origin", "https://console.example")
	rec.Header().Set("Access-Control-Allow-Credentials", "true")

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "export const x = 1;", rec.Body.String())
	require.Equal(t, "text/javascript", rec.Header().Get("Content-Type"))
	require.Len(t, rec.Header().Values("Access-Control-Allow-Origin"), 1, "exactly one ACAO header")
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}

// The override must also apply when the backend writes the body without an
// explicit WriteHeader (implicit 200).
func TestPluginAssetCORSWriter_ImplicitWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &pluginAssetCORSWriter{ResponseWriter: rec}

	_, err := w.Write([]byte("body"))
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}
