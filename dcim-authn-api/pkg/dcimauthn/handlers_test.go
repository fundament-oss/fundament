package dcimauthn

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
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	db "github.com/fundament-oss/fundament/dcim-authn-api/pkg/db/gen"
)

// refreshSubject is the directory entry the stub store resolves; any other
// subject reads as absent, like a soft-deleted user's.
const refreshSubject = "019b4000-1000-7000-8000-000000000001"

// stubStore satisfies Store without a database. Refresh and userinfo are the
// only handlers exercised here, so UserUpsert is never reached.
//
// The directory name deliberately differs from the Name claim the test tokens
// carry ("Alice"): both endpoints must answer from the directory, and with
// equal names a regression to echoing the claims would be invisible.
const directoryName = "Alice Renamed"

type stubStore struct{}

func (stubStore) UserUpsert(context.Context, db.UserUpsertParams) (db.UserUpsertRow, error) {
	panic("UserUpsert not stubbed")
}

func (stubStore) UserGetByID(_ context.Context, arg db.UserGetByIDParams) (db.UserGetByIDRow, error) {
	if arg.ID != uuid.MustParse(refreshSubject) {
		return db.UserGetByIDRow{}, pgx.ErrNoRows
	}
	return db.UserGetByIDRow{ID: arg.ID, Name: directoryName}, nil
}

func refreshTestServer() *Server {
	cfg := &Config{
		JWTSecret:    []byte("test-secret-test-secret-test!!"),
		TokenExpiry:  time.Hour,
		CookieDomain: "localhost",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Server{
		config:        cfg,
		logger:        logger,
		queries:       stubStore{},
		validator:     auth.NewValidator(cfg.JWTSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, logger),
		cookieBuilder: auth.NewCookieBuilder(cfg.CookieDomain, cfg.CookieSecure, auth.DCIMAuthCookieName),
	}
}

func signRefreshToken(t *testing.T, secret []byte, subject string) string {
	t.Helper()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DCIMIssuer,
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Name: "Alice",
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)
	return signed
}

func TestHandleRefresh_ValidToken(t *testing.T) {
	s := refreshTestServer()
	token := signRefreshToken(t, s.config.JWTSecret, refreshSubject)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	cookies := rec.Result().Cookies()
	require.NotEmpty(t, cookies, "expected a refreshed auth cookie to be set")

	// The refreshed token carries the directory's view of the user, not the
	// old token's claims: a rename lands on the next refresh.
	refreshed := decodeToken(t, s.config.JWTSecret, cookies[0].Value)
	assert.Equal(t, refreshSubject, refreshed.Subject)
	assert.Equal(t, directoryName, refreshed.Name)
}

func decodeToken(t *testing.T, secret []byte, token string) *auth.Claims {
	t.Helper()
	claims := &auth.Claims{}
	// Pinned to HS256 like the production validator, so this cannot accept a
	// token the real services would refuse.
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	require.NoError(t, err)
	return claims
}

// A well-signed token whose subject no longer resolves to a live directory
// entry must not be refreshed: a soft-deleted user's session ends in a forced
// re-login, not an extension.
func TestHandleRefresh_UnresolvableSubjectIsRefused(t *testing.T) {
	s := refreshTestServer()
	token := signRefreshToken(t, s.config.JWTSecret, uuid.NewString())

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}

func TestHandleRefresh_InvalidToken(t *testing.T) {
	s := refreshTestServer()

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// /userinfo answers from the directory, not the claims: the frontend probes it
// first and only refreshes when it fails, so this is where a session whose
// subject no longer resolves is turned away instead of running silently
// against a missing entry.
func TestHandleUserInfo_ResolvesDirectoryEntry(t *testing.T) {
	s := refreshTestServer()
	token := signRefreshToken(t, s.config.JWTSecret, refreshSubject)

	req := httptest.NewRequest(http.MethodGet, "/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleUserInfo(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"id": "`+refreshSubject+`", "name": "`+directoryName+`"}`, rec.Body.String())
}

func TestHandleUserInfo_UnresolvableSubjectIsRefused(t *testing.T) {
	s := refreshTestServer()
	token := signRefreshToken(t, s.config.JWTSecret, uuid.NewString())

	req := httptest.NewRequest(http.MethodGet, "/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleUserInfo(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
