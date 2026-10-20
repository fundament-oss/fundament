package dcimauthn

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	db "github.com/fundament-oss/fundament/dcim-authn-api/pkg/db/gen"
	dcimauthnv1 "github.com/fundament-oss/fundament/dcim-authn-api/pkg/proto/gen/dcimauthn/v1"
	"github.com/fundament-oss/fundament/dcim-authn-api/pkg/proto/gen/dcimauthn/v1/dcimauthnv1connect"
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

// outageStore reads as a directory whose database is down: every lookup fails
// with something other than "no such row".
type outageStore struct{ stubStore }

func (outageStore) UserGetByID(context.Context, db.UserGetByIDParams) (db.UserGetByIDRow, error) {
	return db.UserGetByIDRow{}, errors.New("connection refused")
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
		validator:     auth.NewValidatorForAudience(cfg.JWTSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, auth.TokenTypeDCIMUser, logger),
		cookieBuilder: auth.NewCookieBuilder(cfg.CookieDomain, cfg.CookieSecure, auth.DCIMAuthCookieName),
	}
}

func signRefreshToken(t *testing.T, secret []byte, subject string) string {
	t.Helper()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DCIMIssuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{auth.TokenTypeDCIMUser},
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

// A DCIM-issuer token without the dcim-user audience is refused: the audience
// is what keeps other token kinds signed with the shared secret out.
func TestHandleRefresh_MissingAudienceIsRefused(t *testing.T) {
	s := refreshTestServer()

	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DCIMIssuer,
			Subject:   refreshSubject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.config.JWTSecret)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleRefresh_InvalidToken(t *testing.T) {
	s := refreshTestServer()

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// userInfoClient serves the Connect handler over HTTP: authentication lives in
// headers, so a direct method call would bypass the thing most worth testing.
func userInfoClient(t *testing.T, s *Server, token string) dcimauthnv1connect.AuthnServiceClient {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(dcimauthnv1connect.NewAuthnServiceHandler(s))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return dcimauthnv1connect.NewAuthnServiceClient(ts.Client(), ts.URL,
		connect.WithInterceptors(connect.UnaryInterceptorFunc(
			func(next connect.UnaryFunc) connect.UnaryFunc {
				return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
					req.Header().Set("Authorization", "Bearer "+token)
					return next(ctx, req)
				}
			},
		)))
}

// GetUserInfo answers from the directory, not the claims: the frontend probes
// it first and only refreshes when it fails, so this is where a session whose
// subject no longer resolves is turned away instead of running silently
// against a missing entry.
func TestGetUserInfo_ResolvesDirectoryEntry(t *testing.T) {
	s := refreshTestServer()
	client := userInfoClient(t, s, signRefreshToken(t, s.config.JWTSecret, refreshSubject))

	resp, err := client.GetUserInfo(context.Background(), &dcimauthnv1.GetUserInfoRequest{})
	require.NoError(t, err)

	assert.Equal(t, refreshSubject, resp.GetUser().GetId())
	assert.Equal(t, directoryName, resp.GetUser().GetName())
}

func TestGetUserInfo_UnresolvableSubjectIsRefused(t *testing.T) {
	s := refreshTestServer()
	client := userInfoClient(t, s, signRefreshToken(t, s.config.JWTSecret, uuid.NewString()))

	_, err := client.GetUserInfo(context.Background(), &dcimauthnv1.GetUserInfoRequest{})
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// A directory outage is Unavailable, not Unauthenticated: the session is still
// valid, and the frontend keeps its auth hint on this code instead of treating
// the outage as a sign-out.
func TestGetUserInfo_DirectoryOutageIsUnavailable(t *testing.T) {
	s := refreshTestServer()
	s.queries = outageStore{}
	client := userInfoClient(t, s, signRefreshToken(t, s.config.JWTSecret, refreshSubject))

	_, err := client.GetUserInfo(context.Background(), &dcimauthnv1.GetUserInfoRequest{})
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

// The same split on refresh: a database failure answers 503 — the session is
// not over, so the cookie must survive and no fresh token may be minted from
// an unverified subject.
func TestHandleRefresh_DirectoryOutageIs503(t *testing.T) {
	s := refreshTestServer()
	s.queries = outageStore{}
	token := signRefreshToken(t, s.config.JWTSecret, refreshSubject)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}
