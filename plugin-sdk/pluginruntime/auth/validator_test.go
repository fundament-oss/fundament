package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-key-for-testing"

func makeToken(t *testing.T, secret string, claims *Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
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

	tokenStr := makeToken(t, testSecret, &claims)

	result, err := v.ValidateToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, userID, result.UserID)
	require.Len(t, result.OrganizationIDs, 1)
	assert.Equal(t, orgID, result.OrganizationIDs[0])
	assert.Equal(t, "Test User", result.Name)
}

func TestValidateTokenInvalidSecret(t *testing.T) {
	v := NewValidator([]byte(testSecret), nil)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		UserID: uuid.New(),
	}

	tokenStr := makeToken(t, "wrong-secret", &claims)

	_, err := v.ValidateToken(tokenStr)
	assert.Error(t, err)
}

func TestValidateTokenExpired(t *testing.T) {
	v := NewValidator([]byte(testSecret), nil)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
		UserID: uuid.New(),
	}

	tokenStr := makeToken(t, testSecret, &claims)

	_, err := v.ValidateToken(tokenStr)
	assert.Error(t, err)
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

	tokenStr := makeToken(t, testSecret, &claims)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)

	result, err := v.Validate(header)
	require.NoError(t, err)
	assert.Equal(t, userID, result.UserID)
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

	tokenStr := makeToken(t, testSecret, &claims)

	header := http.Header{}
	header.Set("Cookie", AuthCookieName+"="+tokenStr)

	result, err := v.Validate(header)
	require.NoError(t, err)
	assert.Equal(t, userID, result.UserID)
}

func TestValidateNoToken(t *testing.T) {
	v := NewValidator([]byte(testSecret), nil)

	header := http.Header{}
	_, err := v.Validate(header)
	assert.Error(t, err)
}
