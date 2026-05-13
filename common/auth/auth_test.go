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
	return NewValidator(testSecret, nil)
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

func TestClaimsType_ReturnsAudienceAsTokenType(t *testing.T) {
	c := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"fundament-user"},
		},
	}
	if got := c.Type(); got != TokenTypeUser {
		t.Errorf("Type() = %q, want %q", got, TokenTypeUser)
	}
}

func TestClaimsType_EmptyAudienceReturnsEmpty(t *testing.T) {
	c := &Claims{}
	if got := c.Type(); got != "" {
		t.Errorf("Type() = %q, want empty", got)
	}
}

func TestValidatorForAudience_AcceptsMatchingAudience(t *testing.T) {
	userID := uuid.New()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"fundament-user"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenString := signToken(t, claims)

	v := NewValidatorForAudience(testSecret, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatorForAudience_RejectsMismatchedAudience(t *testing.T) {
	userID := uuid.New()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"fundament-plugin"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenString := signToken(t, claims)

	v := NewValidatorForAudience(testSecret, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err == nil {
		t.Fatal("expected error for mismatched audience, got nil")
	}
}

func TestValidatorForAudience_RejectsMissingAudience(t *testing.T) {
	userID := uuid.New()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenString := signToken(t, claims)

	v := NewValidatorForAudience(testSecret, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err == nil {
		t.Fatal("expected error for missing audience, got nil")
	}
}
