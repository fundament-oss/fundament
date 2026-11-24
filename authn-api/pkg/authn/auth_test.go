package authn

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/common/auth"
)

func TestGenerateJWT_SetsUserAudience(t *testing.T) {
	secret := []byte("test-secret")
	server := &AuthnServer{
		config: &Config{
			JWTSecret:     secret,
			TokenExpiry:   15 * time.Minute,
			SessionMaxAge: time.Hour,
		},
	}

	u := &user{
		ID:              uuid.New(),
		OrganizationIDs: []uuid.UUID{uuid.New()},
		Name:            "alice",
	}

	tokenStr, err := server.generateJWT(u, []string{"admin"}, newSession(amrOIDC))
	require.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &auth.Claims{}, func(_ *jwt.Token) (any, error) {
		return secret, nil
	})
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*auth.Claims)
	require.True(t, ok, "claims type assertion failed")
	require.Len(t, claims.Audience, 1)
	require.Equal(t, auth.TokenTypeUser, claims.Audience[0])
	require.NotNil(t, claims.AuthTime)
	assert.WithinDuration(t, time.Now(), claims.AuthTime.Time, time.Minute)
	assert.Equal(t, []string{amrOIDC}, claims.AMR)
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
		validator: auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil),
	}

	path, handler := authnv1connect.NewAuthnServiceHandler(server)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	pluginClaims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypePlugin},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, pluginClaims).SignedString(secret)
	require.NoError(t, err)

	client := authnv1connect.NewAuthnServiceClient(ts.Client(), ts.URL)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+tokenStr)

	_, err = client.GetUserInfo(ctx, &authnv1.GetUserInfoRequest{})
	require.Error(t, err)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// TestGetUserInfo_RejectsWorkloadToken extends the FUN-17 wall to the third
// token type: a WorkloadToken (FUN-22) presented to a user endpoint is
// rejected on the audience pin.
func TestGetUserInfo_RejectsWorkloadToken(t *testing.T) {
	secret := []byte("test-secret")
	server := &AuthnServer{
		config:    &Config{JWTSecret: secret, TokenExpiry: time.Minute},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		validator: auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil),
	}

	path, handler := authnv1connect.NewAuthnServiceHandler(server)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	workloadClaims := &auth.WorkloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeWorkload},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		OrganizationID: uuid.New().String(),
		Workload:       "plugin-controller",
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, workloadClaims).SignedString(secret)
	require.NoError(t, err)

	client := authnv1connect.NewAuthnServiceClient(ts.Client(), ts.URL)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+tokenStr)

	_, err = client.GetUserInfo(ctx, &authnv1.GetUserInfoRequest{})
	require.Error(t, err)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// callGetUserInfo calls GetUserInfo over the real Connect HTTP path with the
// given token as Bearer.
func callGetUserInfo(t *testing.T, server *AuthnServer, token string) (*authnv1.GetUserInfoResponse, error) {
	t.Helper()

	path, handler := authnv1connect.NewAuthnServiceHandler(server)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := authnv1connect.NewAuthnServiceClient(ts.Client(), ts.URL)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	resp, err := client.GetUserInfo(ctx, &authnv1.GetUserInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf("get user info: %w", err)
	}
	return resp, nil
}

// The email address comes from the account, not the token, so a token minted
// without one (as every token is) still shows the address, and a changed
// address shows without waiting for a refresh.
func TestGetUserInfo_ReturnsEmailFromAccount(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice", false)
	_, err := env.superPool.Exec(context.Background(),
		`UPDATE tenant.users SET email = $2 WHERE id = $1`, userID, "alice@example.org")
	require.NoError(t, err)

	resp, err := callGetUserInfo(t, env.server, mintUserToken(t, userID))
	require.NoError(t, err)
	assert.Equal(t, userID.String(), resp.GetUser().GetId())
	assert.Equal(t, "alice@example.org", resp.GetUser().GetEmail())
}

// An account without an address answers with an empty email, which the
// console shows as "Not set".
func TestGetUserInfo_EmptyEmailWhenAccountHasNone(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice", false)

	resp, err := callGetUserInfo(t, env.server, mintUserToken(t, userID))
	require.NoError(t, err)
	assert.Empty(t, resp.GetUser().GetEmail())
}

// A soft-deleted account no longer resolves, so its still-valid token is
// refused, as on refresh.
func TestGetUserInfo_SoftDeletedUserIsRefused(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice", true)

	_, err := callGetUserInfo(t, env.server, mintUserToken(t, userID))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// A database failure still answers from the token, without the email address:
// the console reads any error here as a sign-out.
func TestGetUserInfo_DatabaseOutageAnswersFromToken(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice", false)
	env.servicePool.Close()

	resp, err := callGetUserInfo(t, env.server, mintUserToken(t, userID))
	require.NoError(t, err)
	assert.Equal(t, userID.String(), resp.GetUser().GetId())
	assert.Empty(t, resp.GetUser().GetEmail())
}

// A database that hangs instead of failing also answers from the token, once
// the lookup's deadline passes, rather than holding up the console.
func TestGetUserInfo_HangingDatabaseAnswersFromToken(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice", false)

	// An exclusive lock held by another connection blocks the account lookup
	// until the lock's transaction ends.
	tx, err := env.superPool.Begin(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	_, err = tx.Exec(context.Background(), `LOCK TABLE tenant.users IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)

	start := time.Now()
	resp, err := callGetUserInfo(t, env.server, mintUserToken(t, userID))
	require.NoError(t, err)
	assert.Less(t, time.Since(start), userInfoLookupTimeout+2*time.Second)
	assert.Equal(t, userID.String(), resp.GetUser().GetId())
	assert.Empty(t, resp.GetUser().GetEmail())
}

func TestNew_RejectsInvalidLifetimes(t *testing.T) {
	tests := []struct {
		name          string
		tokenExpiry   time.Duration
		sessionMaxAge time.Duration
		wantErr       string
	}{
		{"session shorter than token", 24 * time.Hour, time.Hour, "shorter than token expiry"},
		{"zero lifetimes", 0, 0, "token expiry must be positive"},
		{"negative token expiry", -time.Hour, time.Hour, "token expiry must be positive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(discardLogger(), &Config{TokenExpiry: tc.tokenExpiry, SessionMaxAge: tc.sessionMaxAge}, nil, nil, nil, nil, nil, nil, nil)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
