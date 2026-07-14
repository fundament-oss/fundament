package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// corsAssetWriter must inject Access-Control-Allow-Origin even when the
// upstream (a real plugin pod via the reverse proxy) does not set it — that
// gap blocked the sandboxed console iframe's ES-module imports in real mode
// (#967). The mock handler sets the header itself; this covers the real path.
func TestCorsAssetWriter_InjectsACAO(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &corsAssetWriter{ResponseWriter: rec}

	// Upstream (rs/cors middleware) left a credentials header; ACAO:* is
	// invalid alongside it, so the writer must clear it.
	w.Header().Set("Content-Type", "text/javascript")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("export const x = 1;"))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"),
		"ACAO:* must not be paired with Access-Control-Allow-Credentials")
	assert.Equal(t, "text/javascript", rec.Header().Get("Content-Type"))
}

// The header must also be set when the handler writes a body without an
// explicit WriteHeader call (implicit 200).
func TestCorsAssetWriter_InjectsACAOOnImplicitWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &corsAssetWriter{ResponseWriter: rec}

	_, _ = w.Write([]byte("<html></html>"))

	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}
