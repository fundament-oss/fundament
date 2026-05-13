package authn

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
)

func TestGenerateJWT_SetsUserAudience(t *testing.T) {
	secret := []byte("test-secret")
	server := &AuthnServer{
		config: &Config{
			JWTSecret:   secret,
			TokenExpiry: 15 * time.Minute,
		},
	}

	u := &user{
		ID:              uuid.New(),
		OrganizationIDs: []uuid.UUID{uuid.New()},
		Name:            "alice",
	}

	tokenStr, err := server.generateJWT(u, []string{"admin"})
	if err != nil {
		t.Fatalf("generateJWT: %v", err)
	}

	parsed, err := jwt.ParseWithClaims(tokenStr, &auth.Claims{}, func(_ *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	claims, ok := parsed.Claims.(*auth.Claims)
	if !ok {
		t.Fatal("claims type assertion failed")
	}
	if got := claims.Type(); got != auth.TokenTypeUser {
		t.Errorf("Type() = %q, want %q", got, auth.TokenTypeUser)
	}
}

func TestAuthnServer_RejectsPluginAudience(t *testing.T) {
	secret := []byte("test-secret")

	// Construct an AuthnServer the way main.go would (without a DB pool).
	// We only exercise the validator field.
	server := &AuthnServer{
		config:    &Config{JWTSecret: secret, TokenExpiry: time.Minute},
		validator: auth.NewValidatorForAudience(secret, auth.TokenTypeUser, nil),
	}

	pluginClaims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypePlugin)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, pluginClaims)
	tokenStr, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	h := http.Header{}
	h.Set("Authorization", "Bearer "+tokenStr)

	if _, err := server.validator.Validate(h); err == nil {
		t.Fatal("expected error for plugin-audience token, got nil")
	}
}
