package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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

func validUserClaims(subject string) *Claims {
	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fundament-authn-api",
			Subject:   subject,
			Audience:  jwt.ClaimStrings{TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

func TestValidateToken_RejectsNonUUIDSubject(t *testing.T) {
	tokenString := signToken(t, validUserClaims("not-a-uuid"))

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
	tokenString := signToken(t, validUserClaims(userID.String()))

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

func TestValidateToken_RejectsMissingExp(t *testing.T) {
	c := validUserClaims(uuid.New().String())
	c.ExpiresAt = nil
	tokenString := signToken(t, c)

	v := newValidator()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	_, err := v.Validate(header)
	require.Error(t, err, "token without exp must be rejected")
}

func TestValidateToken_RejectsWrongIssuer(t *testing.T) {
	c := validUserClaims(uuid.New().String())
	c.Issuer = "evil-issuer"
	tokenString := signToken(t, c)

	v := newValidator()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	_, err := v.Validate(header)
	require.Error(t, err, "token with unexpected issuer must be rejected")
}

func TestValidateToken_RejectsNonHS256Method(t *testing.T) {
	c := validUserClaims(uuid.New().String())
	tok := jwt.NewWithClaims(jwt.SigningMethodHS384, c)
	tokenString, err := tok.SignedString(testSecret)
	require.NoError(t, err)

	v := newValidator()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	_, err = v.Validate(header)
	require.Error(t, err, "non-HS256 signing method must be rejected")
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

func TestValidatorForAudience_AcceptsMatchingAudience(t *testing.T) {
	tokenString := signToken(t, validUserClaims(uuid.New().String()))

	v := NewValidatorForAudience(testSecret, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatorForAudience_RejectsMismatchedAudience(t *testing.T) {
	c := validUserClaims(uuid.New().String())
	c.Audience = jwt.ClaimStrings{TokenTypePlugin}
	tokenString := signToken(t, c)

	v := NewValidatorForAudience(testSecret, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err == nil {
		t.Fatal("expected error for mismatched audience, got nil")
	}
}

// TestValidatorForAudience_AcceptsMultiAudienceWhenExpectedPresent verifies
// that audience matching is set-membership, not first-element equality. JWT
// `aud` is a set per RFC 7519 and tokens may legitimately list more than one
// audience.
func TestValidatorForAudience_AcceptsMultiAudienceWhenExpectedPresent(t *testing.T) {
	c := validUserClaims(uuid.New().String())
	c.Audience = jwt.ClaimStrings{TokenTypePlugin, TokenTypeUser}
	tokenString := signToken(t, c)

	v := NewValidatorForAudience(testSecret, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatorForAudience_RejectsMissingAudience(t *testing.T) {
	c := validUserClaims(uuid.New().String())
	c.Audience = nil
	tokenString := signToken(t, c)

	v := NewValidatorForAudience(testSecret, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenString)

	if _, err := v.Validate(header); err == nil {
		t.Fatal("expected error for missing audience, got nil")
	}
}
