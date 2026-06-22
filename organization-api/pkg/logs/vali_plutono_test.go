package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveValiProxyBase(t *testing.T) {
	t.Run("resolves uid and builds proxy base with basic auth", func(t *testing.T) {
		var gotPath, gotUser, gotPass string
		var gotAuthOK bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotUser, gotPass, gotAuthOK = r.BasicAuth()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uid":"abc123","name":"vali"}`))
		}))
		defer srv.Close()

		base, err := ResolveValiProxyBase(context.Background(), srv.URL, "vali", "observer", "s3cr3t")
		if err != nil {
			t.Fatalf("ResolveValiProxyBase: %v", err)
		}
		if gotPath != "/api/datasources/name/vali" {
			t.Errorf("lookup path = %q, want /api/datasources/name/vali", gotPath)
		}
		if !gotAuthOK || gotUser != "observer" || gotPass != "s3cr3t" {
			t.Errorf("basic auth = (%q, %q, ok=%v)", gotUser, gotPass, gotAuthOK)
		}
		if want := srv.URL + "/api/datasources/proxy/uid/abc123"; base != want {
			t.Errorf("base = %q, want %q", base, want)
		}
	})

	t.Run("error on non-2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		if _, err := ResolveValiProxyBase(context.Background(), srv.URL, "vali", "", ""); err == nil {
			t.Fatal("expected error on 404")
		}
	})

	t.Run("error when uid missing", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"vali"}`))
		}))
		defer srv.Close()

		if _, err := ResolveValiProxyBase(context.Background(), srv.URL, "vali", "", ""); err == nil {
			t.Fatal("expected error when uid is empty")
		}
	})
}
