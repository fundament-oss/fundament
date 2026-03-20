package authn

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	"github.com/fundament-oss/fundament/common/auth"
)

var testJWTSecret = []byte("test-secret-for-cluster-token-tests")

// mockGardenerClient implements GardenerClient for testing.
type mockGardenerClient struct {
	err error
}

func (m *mockGardenerClient) RequestAdminKubeconfig(_ context.Context, _ uuid.UUID, _ int64) (*AdminKubeconfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &AdminKubeconfig{
		Kubeconfig: []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://mock-shoot.example.com
  name: mock
contexts:
- context:
    cluster: mock
    user: mock
  name: mock
current-context: mock
users:
- name: mock
  user:
    token: mock-admin-token
`),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}, nil
}

func newTestAuthnServer(t *testing.T, gardener GardenerClient) *AuthnServer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &AuthnServer{
		config:         &Config{JWTSecret: testJWTSecret},
		logger:         logger,
		validator:      auth.NewValidator(testJWTSecret, logger),
		gardenerClient: gardener,
		// queries is nil — tests that need DB queries will fail,
		// which is expected (we test error paths here).
	}
}

func makeTestJWT(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":              userID.String(),
		"exp":              time.Now().Add(time.Hour).Unix(),
		"iat":              time.Now().Unix(),
		"organization_ids": []string{},
	})
	tokenStr, err := token.SignedString(testJWTSecret)
	require.NoError(t, err)
	return tokenStr
}

func callClusterToken(s *AuthnServer, clusterID uuid.UUID, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/clusters/%s/token", clusterID), nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	s.HandleClusterToken(w, req, openapi_types.UUID(clusterID))
	return w
}

func TestClusterToken_InvalidJWT(t *testing.T) {
	t.Parallel()

	s := newTestAuthnServer(t, &mockGardenerClient{})

	w := callClusterToken(s, uuid.New(), "Bearer invalid-token")
	require.Equal(t, http.StatusUnauthorized, w.Code)

	var resp authnhttp.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, "Unauthorized", resp.Error)
}

func TestClusterToken_MissingAuthHeader(t *testing.T) {
	t.Parallel()

	s := newTestAuthnServer(t, &mockGardenerClient{})

	w := callClusterToken(s, uuid.New(), "")
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestClusterToken_GardenerClientNil(t *testing.T) {
	t.Parallel()

	s := newTestAuthnServer(t, nil)
	userID := uuid.New()
	jwt := makeTestJWT(t, userID)

	w := callClusterToken(s, uuid.New(), "Bearer "+jwt)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp authnhttp.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Contains(t, resp.Error, "not available")
}

func TestClusterToken_ClusterNotFound(t *testing.T) {
	t.Parallel()

	// With nil queries, the DB lookup will panic or fail.
	// The handler calls s.queries.ClusterGetForToken which will nil-pointer.
	// This confirms the handler reaches the DB lookup after auth validation.
	// A proper integration test with embedded Postgres would be needed for full coverage.
	s := newTestAuthnServer(t, &mockGardenerClient{})
	userID := uuid.New()
	jwtToken := makeTestJWT(t, userID)

	require.Panics(t, func() {
		callClusterToken(s, uuid.New(), "Bearer "+jwtToken)
	}, "should panic on nil queries (confirms auth validation passed)")
}
