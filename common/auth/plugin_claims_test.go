package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
			Audience:  jwt.ClaimStrings{TokenTypePlugin},
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

// TestParsePluginToken_AcceptsMultiAudience verifies that audience matching is
// set-membership: a token listing both fundament-user and fundament-plugin in
// its aud claim is accepted by ParsePluginToken regardless of element order.
func TestParsePluginToken_AcceptsMultiAudience(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.Audience = jwt.ClaimStrings{TokenTypeUser, TokenTypePlugin}
	tokenStr := signPluginToken(t, secret, c)

	if _, err := ParsePluginToken(tokenStr, secret); err != nil {
		t.Fatalf("ParsePluginToken: %v", err)
	}
}

func TestParsePluginToken_RejectsUserAudience(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.Audience = jwt.ClaimStrings{TokenTypeUser}
	tokenStr := signPluginToken(t, secret, c)

	if _, err := ParsePluginToken(tokenStr, secret); err == nil {
		t.Fatal("expected error for fundament-user audience, got nil")
	}
}

func TestParsePluginToken_RejectsExpiredToken(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
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

func TestParsePluginToken_RejectsMissingExp(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.ExpiresAt = nil
	tokenStr := signPluginToken(t, secret, c)

	_, err := ParsePluginToken(tokenStr, secret)
	require.Error(t, err, "token without exp must be rejected")
}

func TestParsePluginToken_RejectsWrongIssuer(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.Issuer = "evil-issuer"
	tokenStr := signPluginToken(t, secret, c)

	_, err := ParsePluginToken(tokenStr, secret)
	require.Error(t, err, "token with unexpected issuer must be rejected")
}

func TestParsePluginToken_RejectsNonUUIDSubject(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.Subject = "not-a-uuid"
	tokenStr := signPluginToken(t, secret, c)

	_, err := ParsePluginToken(tokenStr, secret)
	require.Error(t, err, "token with non-UUID subject must be rejected")
}

func TestParsePluginToken_RejectsNonHS256Method(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-hs384-and-hs512")
	c := validPluginClaims()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS384, c)
	tokenStr, err := tok.SignedString(secret)
	require.NoError(t, err)

	_, err = ParsePluginToken(tokenStr, secret)
	require.Error(t, err, "non-HS256 signing method must be rejected")
}
