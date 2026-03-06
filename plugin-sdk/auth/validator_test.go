package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "test-secret-key-for-testing"

func makeToken(t *testing.T, secret string, claims Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestValidateToken(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()

	v := NewValidator([]byte(testSecret), nil)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID:          userID,
		OrganizationIDs: []uuid.UUID{orgID},
		Name:            "Test User",
	}

	tokenStr := makeToken(t, testSecret, claims)

	result, err := v.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UserID != userID {
		t.Fatalf("expected user ID %s, got %s", userID, result.UserID)
	}
	if len(result.OrganizationIDs) != 1 || result.OrganizationIDs[0] != orgID {
		t.Fatalf("unexpected organization IDs: %v", result.OrganizationIDs)
	}
	if result.Name != "Test User" {
		t.Fatalf("expected name 'Test User', got %q", result.Name)
	}
}

func TestValidateTokenInvalidSecret(t *testing.T) {
	v := NewValidator([]byte(testSecret), nil)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		UserID: uuid.New(),
	}

	tokenStr := makeToken(t, "wrong-secret", claims)

	_, err := v.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for invalid secret")
	}
}

func TestValidateTokenExpired(t *testing.T) {
	v := NewValidator([]byte(testSecret), nil)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
		UserID: uuid.New(),
	}

	tokenStr := makeToken(t, testSecret, claims)

	_, err := v.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateFromBearerHeader(t *testing.T) {
	userID := uuid.New()
	v := NewValidator([]byte(testSecret), nil)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		UserID: userID,
	}

	tokenStr := makeToken(t, testSecret, claims)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)

	result, err := v.Validate(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UserID != userID {
		t.Fatalf("expected user ID %s, got %s", userID, result.UserID)
	}
}

func TestValidateFromCookie(t *testing.T) {
	userID := uuid.New()
	v := NewValidator([]byte(testSecret), nil)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		UserID: userID,
	}

	tokenStr := makeToken(t, testSecret, claims)

	header := http.Header{}
	header.Set("Cookie", AuthCookieName+"="+tokenStr)

	result, err := v.Validate(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UserID != userID {
		t.Fatalf("expected user ID %s, got %s", userID, result.UserID)
	}
}

func TestValidateNoToken(t *testing.T) {
	v := NewValidator([]byte(testSecret), nil)

	header := http.Header{}
	_, err := v.Validate(header)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}
