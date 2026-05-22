package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func signPluginToken(t *testing.T, secret []byte, c *PluginClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func validPluginClaims() *PluginClaims {
	return &PluginClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fundament-authn-api",
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{string(TokenTypePlugin)},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ClusterID:      uuid.New().String(),
		InstallationID: uuid.New().String(),
		PluginName:     "cert-manager",
		PluginVersion:  "v1.17.2",
		DefinitionHash: "sha256:1f3c",
	}
}

func TestPluginClaims_Type(t *testing.T) {
	c := validPluginClaims()
	if got := c.Type(); got != TokenTypePlugin {
		t.Errorf("Type() = %q, want %q", got, TokenTypePlugin)
	}
	empty := &PluginClaims{}
	if got := empty.Type(); got != "" {
		t.Errorf("Type() on empty = %q, want empty", got)
	}
}

func TestParsePluginToken_AcceptsValidToken(t *testing.T) {
	secret := []byte("test-secret")
	want := validPluginClaims()
	tokenStr := signPluginToken(t, secret, want)

	got, err := ParsePluginToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("ParsePluginToken: %v", err)
	}
	if got.ClusterID != want.ClusterID {
		t.Errorf("ClusterID = %q, want %q", got.ClusterID, want.ClusterID)
	}
	if got.InstallationID != want.InstallationID {
		t.Errorf("InstallationID = %q, want %q", got.InstallationID, want.InstallationID)
	}
	if got.PluginName != "cert-manager" || got.PluginVersion != "v1.17.2" {
		t.Errorf("plugin identity = %q/%q", got.PluginName, got.PluginVersion)
	}
	if got.DefinitionHash != "sha256:1f3c" {
		t.Errorf("DefinitionHash = %q", got.DefinitionHash)
	}
}

func TestParsePluginToken_RejectsUserAudience(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.Audience = jwt.ClaimStrings{string(TokenTypeUser)}
	tokenStr := signPluginToken(t, secret, c)

	if _, err := ParsePluginToken(tokenStr, secret); err == nil {
		t.Fatal("expected error for fundament-user audience, got nil")
	}
}

func TestParsePluginToken_RejectsExpiredToken(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	tokenStr := signPluginToken(t, secret, c)

	if _, err := ParsePluginToken(tokenStr, secret); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestParsePluginToken_RejectsWrongSecret(t *testing.T) {
	tokenStr := signPluginToken(t, []byte("secret-a"), validPluginClaims())
	if _, err := ParsePluginToken(tokenStr, []byte("secret-b")); err == nil {
		t.Fatal("expected error for wrong signing secret, got nil")
	}
}
