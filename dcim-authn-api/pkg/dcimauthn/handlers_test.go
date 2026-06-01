package dcimauthn

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/fundament-oss/fundament/common/auth"
)

func refreshTestServer(maxSessionAge time.Duration) *Server {
	cfg := &Config{
		JWTSecret:     []byte("test-secret-test-secret-test!!"),
		TokenExpiry:   time.Hour,
		MaxSessionAge: maxSessionAge,
		CookieDomain:  "localhost",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Server{
		config:        cfg,
		logger:        logger,
		validator:     auth.NewValidator(cfg.JWTSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, logger),
		cookieBuilder: auth.NewCookieBuilder(cfg.CookieDomain, cfg.CookieSecure, auth.DCIMAuthCookieName),
	}
}

func signRefreshToken(t *testing.T, secret []byte, authTime *jwt.NumericDate) string {
	t.Helper()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DCIMIssuer,
			Subject:   "019b4000-1000-7000-8000-000000000001",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Name:     "Alice",
		AuthTime: authTime,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

func TestHandleRefresh_WithinLifetime(t *testing.T) {
	s := refreshTestServer(168 * time.Hour)
	token := signRefreshToken(t, s.config.JWTSecret, jwt.NewNumericDate(time.Now().Add(-time.Hour)))

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

func TestHandleRefresh_ExceedsMaxSessionAge(t *testing.T) {
	s := refreshTestServer(time.Hour)
	token := signRefreshToken(t, s.config.JWTSecret, jwt.NewNumericDate(time.Now().Add(-2*time.Hour)))

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for session past max lifetime", rec.Code)
	}
}

func TestHandleRefresh_MissingAuthTime(t *testing.T) {
	s := refreshTestServer(168 * time.Hour)
	token := signRefreshToken(t, s.config.JWTSecret, nil)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.HandleRefresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for token without auth_time", rec.Code)
	}
}
