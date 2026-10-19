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

	"github.com/fundament-oss/fundament/common/auth"
	db "github.com/fundament-oss/fundament/dcim-authn-api/pkg/db/gen"
)

// refreshSubject is the directory entry the stub store resolves; any other
// subject reads as absent, like a pre-rollover external-ref subject or a
// soft-deleted user.
const refreshSubject = "019b4000-1000-7000-8000-000000000001"

// stubStore satisfies Store without a database. Refresh is the only handler
// exercised here, so UserUpsert is never reached.
type stubStore struct{}

func (stubStore) UserUpsert(context.Context, db.UserUpsertParams) (db.UserUpsertRow, error) {
	panic("UserUpsert not stubbed")
}

func (stubStore) UserGetByID(_ context.Context, arg db.UserGetByIDParams) (db.UserGetByIDRow, error) {
	if arg.ID != uuid.MustParse(refreshSubject) {
		return db.UserGetByIDRow{}, pgx.ErrNoRows
	}
	return db.UserGetByIDRow{ID: arg.ID, Name: "Alice"}, nil
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
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

func TestHandleRefresh_ValidToken(t *testing.T) {
	s := refreshTestServer()
	token := signRefreshToken(t, s.config.JWTSecret, refreshSubject)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a refreshed auth cookie to be set")
	}
}

// A well-signed token whose subject no longer resolves to a live directory
// entry must not be refreshed: this is what retires pre-rollover tokens (whose
// subject is an external reference, not dcim.users.id) and the sessions of
// soft-deleted users, forcing a fresh login.
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

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for invalid token", rec.Code)
	}
}
