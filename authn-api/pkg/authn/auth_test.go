package authn

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/common/auth"
)

func TestGenerateJWT_SetsUserAudience(t *testing.T) {
	secret := []byte("test-secret")
	server := &AuthnServer{
		config: &Config{
			JWTSecret:   secret,
			TokenExpiry: 15 * time.Minute,
		},
	}

	u := &user{
		ID:              uuid.New(),
		OrganizationIDs: []uuid.UUID{uuid.New()},
		Name:            "alice",
	}

	tokenStr, err := server.generateJWT(u, []string{"admin"})
	require.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &auth.Claims{}, func(_ *jwt.Token) (any, error) {
		return secret, nil
	})
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*auth.Claims)
	require.True(t, ok, "claims type assertion failed")
	require.Len(t, claims.Audience, 1)
	require.Equal(t, auth.TokenTypeUser, claims.Audience[0])
}

// TestGetUserInfo_RejectsPluginToken verifies that a PluginToken presented to
// GetUserInfo over the real Connect HTTP path is rejected with Unauthenticated.
// This exercises the wire-up between the Connect handler and the audience-aware
// validator end to end, not just the validator field in isolation.
func TestGetUserInfo_RejectsPluginToken(t *testing.T) {
	secret := []byte("test-secret")

	// GetUserInfo only reads s.validator before returning; constructing
	// AuthnServer with only the fields it touches lets the test stay
	// DB-less. The full authn.New constructor requires a real Postgres pool.
	server := &AuthnServer{
		config:    &Config{JWTSecret: secret, TokenExpiry: time.Minute},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		validator: auth.NewValidatorForAudience(secret, auth.TokenTypeUser, nil),
	}

	path, handler := authnv1connect.NewAuthnServiceHandler(server)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
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

	client := authnv1connect.NewAuthnServiceClient(ts.Client(), ts.URL)
	req := connect.NewRequest(&authnv1.GetUserInfoRequest{})
	req.Header().Set("Authorization", "Bearer "+tokenStr)

	_, err = client.GetUserInfo(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
