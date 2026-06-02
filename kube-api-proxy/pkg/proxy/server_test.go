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
			Audience:  jwt.ClaimStrings{string(auth.TokenTypePlugin)},
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
