package proxy_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/proxy"
)

// newMockServer builds a mock-mode proxy with no OpenFGA client.
func newMockServer(t *testing.T, cfg *proxy.Config) *httptest.Server {
	t.Helper()
	if cfg.Mode == "" {
		cfg.Mode = "mock"
	}
	srv, err := proxy.New(slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, nil, nil)
	require.NoError(t, err)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestClusterProxy_RequestGating exercises the cluster-route gates that
// short-circuit before authorization, so they need no OpenFGA client. These
// are the CI-runnable interface equivalents of the removed real-Gardener
// negative scenarios.
func TestClusterProxy_RequestGating(t *testing.T) {
	ts := newMockServer(t, &proxy.Config{JWTSecret: []byte("test-secret")})
	validID := uuid.NewString()

	tests := []struct {
		name string
		path string
		want int
	}{
		{"non-UUID cluster ID is a bad request", "/clusters/not-a-uuid/api/v1/namespaces", http.StatusBadRequest},
		{"path outside the Kubernetes API is not found", "/clusters/" + validID + "/metrics", http.StatusNotFound},
		{"missing token on an API path is unauthorized", "/clusters/" + validID + "/api/v1/namespaces", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+tc.path, http.NoBody)
			require.NoError(t, err)
			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			assert.Equal(t, tc.want, resp.StatusCode)
		})
	}
}

// TestClusterProxy_RejectsPluginToken verifies that a PluginToken presented to
// the cluster proxy is rejected at authentication with 401. This exercises the
// full HTTP path through the audience-aware validator wired in proxy.New.
func TestClusterProxy_RejectsPluginToken(t *testing.T) {
	secret := []byte("test-secret")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv, err := proxy.New(logger, &proxy.Config{
		JWTSecret: secret,
		Mode:      "mock",
	}, nil, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	pluginClaims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypePlugin},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, pluginClaims).SignedString(secret)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/clusters/"+uuid.NewString()+"/api/v1/namespaces", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestClusterProxy_CookieOrigin checks that a cookie-borne UserToken is honored
// only from an allowed Origin, while a bearer token works from any origin.
func TestClusterProxy_CookieOrigin(t *testing.T) {
	const consoleOrigin = "https://console.example.com"
	const pluginOrigin = "https://plugin-proxy.example.com"
	secret := []byte("test-secret")
	ts := newMockServer(t, &proxy.Config{JWTSecret: secret, CookieAllowedOrigins: []string{consoleOrigin + "/"}})

	userClaims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			Issuer:    auth.ConsoleIssuer,
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, userClaims).SignedString(secret)
	require.NoError(t, err)

	tests := []struct {
		name    string
		origin  string
		authz   string
		cookie  bool
		upgrade bool
		want    int
	}{
		{name: "cookie from the console origin", origin: consoleOrigin, cookie: true, want: http.StatusOK},
		{name: "cookie from the plugin-proxy origin", origin: pluginOrigin, cookie: true, want: http.StatusUnauthorized},
		{name: "cookie without Origin", cookie: true, want: http.StatusUnauthorized},
		{name: "cookie on a WebSocket upgrade from a foreign origin", origin: pluginOrigin, cookie: true, upgrade: true, want: http.StatusUnauthorized},
		{name: "cookie with a non-bearer Authorization header", origin: pluginOrigin, authz: "Basic Zm9vOmJhcg==", cookie: true, want: http.StatusUnauthorized},
		{name: "bearer from a foreign origin", origin: pluginOrigin, authz: "Bearer " + token, want: http.StatusOK},
		{name: "bearer without Origin", authz: "Bearer " + token, want: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/clusters/"+uuid.NewString()+"/api/v1/namespaces", http.NoBody)
			require.NoError(t, err)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.authz != "" {
				req.Header.Set("Authorization", tc.authz)
			}
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: auth.ConsoleAuthCookieName, Value: token}) //nolint:gosec // request cookie in a test
			}
			if tc.upgrade {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			assert.Equal(t, tc.want, resp.StatusCode)
		})
	}
}

// TestClusterProxy_CookieRefusedWithoutAllowedOrigins checks that an empty
// allow-list refuses every cookie request, even one carrying an Origin.
func TestClusterProxy_CookieRefusedWithoutAllowedOrigins(t *testing.T) {
	secret := []byte("test-secret")
	ts := newMockServer(t, &proxy.Config{JWTSecret: secret})

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			Issuer:    auth.ConsoleIssuer,
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}).SignedString(secret)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/clusters/"+uuid.NewString()+"/api/v1/namespaces", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://console.example.com")
	req.AddCookie(&http.Cookie{Name: auth.ConsoleAuthCookieName, Value: token}) //nolint:gosec // request cookie in a test

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
