package discovery

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve starts a server answering the discovery path with body, in which
// ISSUER stands for the server's own URL.
func serve(t *testing.T, contentType, body string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != Path {
			http.NotFound(w, r)
			return
		}
		assert.Empty(t, r.Header.Get("Authorization"), "discovery must not send credentials")
		w.Header().Set("Content-Type", contentType)
		_, _ = fmt.Fprint(w, strings.ReplaceAll(body, "ISSUER", server.URL))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetch(t *testing.T) {
	server := serve(t, "application/json; charset=utf-8", `{
		"issuer": "ISSUER/",
		"organization_api": "https://organization.example.com/",
		"authn_api": "https://authn.example.com",
		"kube_api_proxy": "",
		"some_future_api": "https://future.example.com"
	}`)

	doc, err := Fetch(t.Context(), nil, server.URL+"/")
	require.NoError(t, err)

	assert.Equal(t, "https://organization.example.com", doc.OrganizationAPI)
	assert.Equal(t, "https://authn.example.com", doc.AuthnAPI)
	assert.Empty(t, doc.KubeAPIProxy)
	assert.Empty(t, doc.MarketplaceRegistryAPI)
}

func TestFetchRejects(t *testing.T) {
	tests := map[string]struct {
		contentType string
		body        string
		wantErr     string
	}{
		"wrong content type": {
			contentType: "text/html",
			body:        `{"issuer": "ISSUER"}`,
			wantErr:     "is not application/json",
		},
		"invalid json": {
			contentType: "application/json",
			body:        `<html>`,
			wantErr:     "parsing",
		},
		"issuer of another installation": {
			contentType: "application/json",
			body:        `{"issuer": "https://other.example.com"}`,
			wantErr:     "does not match",
		},
		"plain http service": {
			contentType: "application/json",
			body:        `{"issuer": "ISSUER", "authn_api": "http://authn.example.com"}`,
			wantErr:     "authn_api",
		},
		"relative service url": {
			contentType: "application/json",
			body:        `{"issuer": "ISSUER", "organization_api": "/api"}`,
			wantErr:     "organization_api",
		},
		"oversized document": {
			contentType: "application/json",
			body:        `{"issuer": "ISSUER", "pad": "` + strings.Repeat("x", maxBodyBytes) + `"}`,
			wantErr:     "larger than",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			server := serve(t, tt.contentType, tt.body)

			_, err := Fetch(t.Context(), nil, server.URL)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestFetchNotFound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	_, err := Fetch(t.Context(), nil, server.URL)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestFetchRefusesCrossOriginRedirect(t *testing.T) {
	target := serve(t, "application/json", `{"issuer": "ISSUER"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+Path, http.StatusFound)
	}))
	t.Cleanup(server.Close)

	_, err := Fetch(t.Context(), nil, server.URL)
	require.ErrorContains(t, err, "redirect to another origin")
}

func TestFetchStopsRedirectLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, Path, http.StatusFound)
	}))
	t.Cleanup(server.Close)

	// No timeout: only the redirect limit can end this.
	_, err := Fetch(t.Context(), &http.Client{}, server.URL)
	require.ErrorContains(t, err, "stopped after 10 redirects")
}

func TestFetchComparesIssuerByOrigin(t *testing.T) {
	var port string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer": "http://LOCALHOST:%s"}`, port)
	}))
	t.Cleanup(server.Close)
	port = strings.TrimPrefix(server.URL, "http://127.0.0.1:")

	// Host case is not significant, in either direction.
	_, err := Fetch(t.Context(), nil, "http://localhost:"+port)
	require.NoError(t, err)

	// A different host is still refused.
	_, err = Fetch(t.Context(), nil, server.URL)
	require.ErrorContains(t, err, "does not match")
}

func TestCanonicalOrigin(t *testing.T) {
	tests := map[string]string{
		"https://Example.COM":      "https://example.com",
		"https://example.com:443":  "https://example.com",
		"http://example.com:80":    "http://example.com",
		"https://example.com:8443": "https://example.com:8443",
		"http://[::1]:8080":        "http://[::1]:8080",
		"http://[::1]":             "http://[::1]",
	}

	for raw, want := range tests {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		assert.Equal(t, want, canonicalOrigin(u), raw)
	}
}

func TestParseServiceURLHTTPOnlyForLocalInstallations(t *testing.T) {
	_, err := parseServiceURL("http://localhost:9999", false)
	require.ErrorContains(t, err, "http is only allowed")

	got, err := parseServiceURL("http://localhost:9999/", true)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:9999", got)
}

func TestFetchRejectsBaseURL(t *testing.T) {
	tests := map[string]string{
		"plain http":  "http://fundament.example.com",
		"with a path": "https://fundament.example.com/tenant",
		"no scheme":   "fundament.example.com",
		"with query":  "https://fundament.example.com?x=1",
		"userinfo":    "https://someone@fundament.example.com",
		"empty query": "https://fundament.example.com?",
	}

	for name, baseURL := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Fetch(t.Context(), nil, baseURL)
			require.ErrorContains(t, err, "invalid installation URL")
		})
	}
}

func TestFetchSendsNoCookies(t *testing.T) {
	var gotCookie string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer": %q}`, server.URL)
	}))
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	jar.SetCookies(serverURL, []*http.Cookie{{Name: "session", Value: "secret", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}})

	_, err = Fetch(t.Context(), &http.Client{Jar: jar}, server.URL)
	require.NoError(t, err)
	assert.Empty(t, gotCookie)
}

func TestFetchRefusesRedirectWithCredentials(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == Path {
			http.Redirect(w, r, "http://user:pw@"+strings.TrimPrefix(server.URL, "http://")+"/elsewhere", http.StatusFound)
			return
		}
		assert.Empty(t, r.Header.Get("Authorization"))
	}))
	t.Cleanup(server.Close)

	_, err := Fetch(t.Context(), nil, server.URL)
	require.ErrorContains(t, err, "redirect with credentials refused")
}

func TestFetchRejectsIssuerWithQueryFragmentOrUserinfo(t *testing.T) {
	for _, issuer := range []string{"ISSUER?x=1", "ISSUER#frag", "ISSUER?"} {
		server := serve(t, "application/json", `{"issuer": "`+issuer+`"}`)

		_, err := Fetch(t.Context(), nil, server.URL)
		require.ErrorContains(t, err, "does not match", issuer)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer": "http://someone@%s"}`, strings.TrimPrefix(server.URL, "http://"))
	}))
	t.Cleanup(server.Close)
	_, err := Fetch(t.Context(), nil, server.URL)
	require.ErrorContains(t, err, "does not match")
}
