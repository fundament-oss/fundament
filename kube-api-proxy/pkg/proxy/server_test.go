package proxy_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/proxy"
)

// Plugin console assets are public and unauthenticated; without CONSOLE_ORIGINS and
// PUBLIC_ORIGIN they are served with no CSP and an unchecked ?host=, which is script
// execution on this origin for anyone who can get a victim to open a crafted link.
// A real deployment must not be able to come up in that state by omission.
func TestNew_RealModeRequiresConsoleAssetOrigins(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tc := range []struct {
		name           string
		consoleOrigins []string
		publicOrigin   string
	}{
		{name: "neither"},
		{name: "no public origin", consoleOrigins: []string{"https://console.example"}},
		{name: "no console origins", publicOrigin: "https://k8s-api.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := proxy.New(logger, &proxy.Config{
				JWTSecret:      []byte("test-secret"),
				Mode:           "real",
				ConsoleOrigins: tc.consoleOrigins,
				PublicOrigin:   tc.publicOrigin,
			}, nil)

			require.ErrorContains(t, err, "CONSOLE_ORIGINS and PUBLIC_ORIGIN are required")
		})
	}
}

// Mock mode is a developer's laptop: a half-configured policy stands down (with a
// warning) rather than refusing to start.
func TestNew_MockModeToleratesMissingConsoleAssetOrigins(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := proxy.New(logger, &proxy.Config{JWTSecret: []byte("test-secret"), Mode: "mock"}, nil)

	require.NoError(t, err)
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
	}, nil)
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

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/clusters/"+uuid.NewString()+"/api/v1/namespaces", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
