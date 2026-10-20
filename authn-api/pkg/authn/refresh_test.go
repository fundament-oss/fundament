package authn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/auth"
)

// These tests run HandleRefresh against a real database as fun_authn_api:
// refresh is a security boundary (a soft-deleted user must not extend their
// session) and its answers split three ways — 401 ends the session, 503 says
// the session is fine but the directory is not, 200 re-signs from the account.

var refreshTestSecret = []byte("refresh-test-secret-refresh-test")

func createTestDatabase(t *testing.T, name string) {
	t.Helper()

	adminURL := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort)

	adminPool, err := pgxpool.New(context.Background(), adminURL)
	require.NoError(t, err)
	defer adminPool.Close()

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)
}

func testNameToDBName(testName string) string {
	name := strings.ToLower(testName)
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")

	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

// refreshEnv is a per-test database cloned from the fundament template plus an
// AuthnServer wired to it: superPool seeds as postgres, the server's queries
// run as fun_authn_api so every lookup goes through the role's grants and RLS.
type refreshEnv struct {
	superPool   *pgxpool.Pool
	servicePool *pgxpool.Pool
	server      *AuthnServer
}

func newRefreshEnv(t *testing.T) *refreshEnv {
	t.Helper()

	dbName := testNameToDBName(t.Name())
	createTestDatabase(t, dbName)

	superPool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(superPool.Close)

	servicePool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"postgres://fun_authn_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(servicePool.Close)

	cfg := &Config{JWTSecret: refreshTestSecret, TokenExpiry: time.Hour, CookieDomain: "localhost"}
	logger := discardLogger()
	server := &AuthnServer{
		config:        cfg,
		logger:        logger,
		queries:       db.New(servicePool),
		validator:     auth.NewValidatorForAudience(cfg.JWTSecret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, logger),
		cookieBuilder: auth.NewCookieBuilder(cfg.CookieDomain, cfg.CookieSecure, auth.ConsoleAuthCookieName),
	}

	return &refreshEnv{superPool: superPool, servicePool: servicePool, server: server}
}

func seedUser(t *testing.T, env *refreshEnv, name string, deleted bool) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := env.superPool.QueryRow(context.Background(),
		`INSERT INTO tenant.users (name, deleted) VALUES ($1, CASE WHEN $2 THEN now() END) RETURNING id`,
		name, deleted).Scan(&id)
	require.NoError(t, err)
	return id
}

// mintUserToken mirrors generateJWT: a console user token whose Name claim is
// deliberately stale, so a 200 can only mean the account was re-read.
func mintUserToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Name: "Stale Name",
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(refreshTestSecret)
	require.NoError(t, err)
	return signed
}

func postRefresh(s *AuthnServer, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.HandleRefresh(rec, req)
	return rec
}

// The refreshed token carries the account's view of the user, not the old
// token's claims: a rename lands on the next refresh.
func TestHandleRefresh_RenameLandsOnRefresh(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice Renamed", false)

	rec := postRefresh(env.server, mintUserToken(t, userID))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp authnhttp.RefreshResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	claims := &auth.Claims{}
	_, err := jwt.ParseWithClaims(resp.AccessToken, claims, func(*jwt.Token) (any, error) { return refreshTestSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	require.NoError(t, err)
	assert.Equal(t, userID.String(), claims.Subject)
	assert.Equal(t, "Alice Renamed", claims.Name)
}

// A soft-deleted user's well-signed token must not be refreshed: the session
// ends in a forced re-login, not an indefinite extension.
func TestHandleRefresh_SoftDeletedUserIsRefused(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice", true)

	rec := postRefresh(env.server, mintUserToken(t, userID))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}

// A database failure is 503, not 401: the session is still valid and the
// client must not treat the outage as a sign-out.
func TestHandleRefresh_DatabaseOutageIs503(t *testing.T) {
	env := newRefreshEnv(t)
	userID := seedUser(t, env, "Alice", false)
	env.servicePool.Close()

	rec := postRefresh(env.server, mintUserToken(t, userID))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}
