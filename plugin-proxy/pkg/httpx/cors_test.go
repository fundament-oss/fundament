package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestCORS_AnswersPreflight(t *testing.T) {
	h := WithCORS("https://plugin-proxy.test", []string{"GET", "POST"}, okHandler())
	r := httptest.NewRequest(http.MethodOptions, "/installations/x/runtime/ping", nil)
	r.Header.Set("Origin", "https://plugin-proxy.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight code = %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://plugin-proxy.test" {
		t.Errorf("ACAO = %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Errorf("ACAH missing")
	}
}

func TestCORS_DecoratesActualRequest(t *testing.T) {
	h := WithCORS("https://plugin-proxy.test", []string{"GET"}, okHandler())
	r := httptest.NewRequest(http.MethodGet, "/installations/x/runtime/ping", nil)
	r.Header.Set("Origin", "https://plugin-proxy.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("code = %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://plugin-proxy.test" {
		t.Errorf("ACAO = %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_IgnoresUnknownOrigin(t *testing.T) {
	h := WithCORS("https://plugin-proxy.test", []string{"GET"}, okHandler())
	r := httptest.NewRequest(http.MethodGet, "/installations/x/runtime/ping", nil)
	r.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("ACAO must be empty for unknown origin, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}
