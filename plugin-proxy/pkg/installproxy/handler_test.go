package installproxy

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
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func mintPluginToken(t *testing.T, secret []byte, installID, clusterID string) string {
	t.Helper()
	c := &auth.PluginClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    auth.ConsoleIssuer,
			Audience:  jwt.ClaimStrings{auth.TokenTypePlugin},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		ClusterID:      clusterID,
		InstallationID: installID,
		PluginName:     "cert-manager",
		PluginVersion:  "v1.17.2",
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	require.NoError(t, err, "sign")
	return s
}

// allowAuthz / denyAuthz implement the OpenFGA can_view check.
type allowAuthz struct{}

func (allowAuthz) CanViewCluster(context.Context, string, string) (bool, error) { return true, nil }

type denyAuthz struct{}

func (denyAuthz) CanViewCluster(context.Context, string, string) (bool, error) { return false, nil }

func stubBackend() Backend {
	return BackendFunc(func(w http.ResponseWriter, r *http.Request, _ Route) {
		//nolint:gosec // test stub: response body is a fixed string concatenated with the test-controlled URL path.
		_, _ = w.Write([]byte("pong " + r.URL.Path))
	})
}

func TestRuntimeProxy_RejectsMissingToken(t *testing.T) {
	h := New([]byte("s"), allowAuthz{}, stubBackend(), discardLogger())
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/installations/abc/runtime/api/ping", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRuntimeProxy_RejectsInstallationIDMismatch(t *testing.T) {
	secret := []byte("s")
	tok := mintPluginToken(t, secret, "INSTALL-X", "CLUSTER-X")
	h := New(secret, allowAuthz{}, stubBackend(), discardLogger())
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/installations/INSTALL-Y/runtime/api/ping", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code, "body = %s", w.Body.String())
}

func TestRuntimeProxy_RejectsWhenCanViewFalse(t *testing.T) {
	secret := []byte("s")
	tok := mintPluginToken(t, secret, "INSTALL-X", "CLUSTER-X")
	h := New(secret, denyAuthz{}, stubBackend(), discardLogger())
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/installations/INSTALL-X/runtime/api/ping", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRuntimeProxy_ForwardsAuthorizedRequest(t *testing.T) {
	secret := []byte("s")
	tok := mintPluginToken(t, secret, "INSTALL-X", "CLUSTER-X")
	h := New(secret, allowAuthz{}, stubBackend(), discardLogger())
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/installations/INSTALL-X/runtime/api/ping", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "pong")
}

func TestParseRoute(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		wantOK bool
	}{
		{"runtime ok", "/installations/INSTALL-X/runtime/api/ping", true},
		{"controller ok", "/installations/INSTALL-X/controller/v1/health", true},
		{"missing tail", "/installations/INSTALL-X/runtime/", false},
		{"empty install id", "/installations//runtime/api/ping", false},
		{"unknown kind", "/installations/INSTALL-X/admin/api/ping", false},
		{"missing prefix", "/foo/INSTALL-X/runtime/api/ping", false},
		{"traversal in tail", "/installations/INSTALL-X/runtime/../../../api/v1/secrets", false},
		{"traversal in install id", "/installations/../runtime/api/ping", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := parseRoute(tc.path)
			assert.Equal(t, tc.wantOK, ok, "parseRoute(%q)", tc.path)
		})
	}
}
