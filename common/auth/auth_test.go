package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var testSecret = []byte("test-secret")

func signToken(t *testing.T, claims *Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(testSecret)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return s
}

func newValidator() *Validator {
	return NewValidator(testSecret, ConsoleAuthCookieName, ConsoleIssuer, nil)
}

func TestValidateToken_RejectsNonUUIDSubject(t *testing.T) {
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "not-a-uuid",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenString := signToken(t, claims)

	v := newValidator()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	_, err := v.Validate(header)
	if err == nil {
		t.Fatal("expected error for non-UUID subject, got nil")
	}
}

func TestValidateToken_AcceptsValidUUIDSubject(t *testing.T) {
	userID := uuid.New()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    ConsoleIssuer,
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenString := signToken(t, claims)

	v := newValidator()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	got, err := v.Validate(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Subject != userID.String() {
		t.Errorf("subject = %q, want %q", got.Subject, userID.String())
	}
}

func TestValidateToken_RejectsForeignIssuer(t *testing.T) {
	// A token minted by another service that shares the JWT secret (e.g. DCIM)
	// must not be accepted by a validator pinned to the console issuer, even
	// though the signature and subject are valid.
	userID := uuid.New()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    DCIMIssuer,
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenString := signToken(t, claims)

	v := newValidator()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err == nil {
		t.Fatal("expected error for foreign issuer, got nil")
	}
}

func TestValidateToken_RejectsMissingIssuer(t *testing.T) {
	userID := uuid.New()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenString := signToken(t, claims)

	v := newValidator()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err == nil {
		t.Fatal("expected error for missing issuer, got nil")
	}
}

func TestClaimsUserID(t *testing.T) {
	userID := uuid.New()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(),
		},
	}
	if got := claims.UserID(); got != userID {
		t.Errorf("UserID() = %v, want %v", got, userID)
	}
}
