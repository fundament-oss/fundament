package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
)

// TestNew_BuildsAudienceAwareValidator verifies that kube-api-proxy's Server
// construction wires up a validator that rejects PluginTokens. This guards
// against regression of the constructor wiring.
func TestNew_BuildsAudienceAwareValidator(t *testing.T) {
	secret := []byte("test-secret")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv, err := New(logger, &Config{
		JWTSecret: secret,
		Mode:      "mock",
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
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

	if _, err := srv.authValidator.Validate(h); err == nil {
		t.Fatal("expected validator to reject plugin-aud token, got nil")
	}
}
