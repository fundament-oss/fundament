package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPluginName    = "test-plugin"
	testPluginVersion = "v1.17.2"
	testDefHash       = "sha256:1f3c"
)

func signPluginToken(t *testing.T, secret []byte, c *PluginClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	s, err := tok.SignedString(secret)
	require.NoError(t, err, "sign")
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
		PluginName:     testPluginName,
		PluginVersion:  testPluginVersion,
		DefinitionHash: testDefHash,
	}
}

func TestParsePluginToken_AcceptsValidToken(t *testing.T) {
	secret := []byte("test-secret")
	want := validPluginClaims()
	tokenStr := signPluginToken(t, secret, want)

	got, err := ParsePluginToken(tokenStr, secret)
	require.NoError(t, err, "ParsePluginToken")
	assert.Equal(t, want.ClusterID, got.ClusterID)
	assert.Equal(t, want.InstallationID, got.InstallationID)
	assert.Equal(t, testPluginName, got.PluginName)
	assert.Equal(t, testPluginVersion, got.PluginVersion)
	assert.Equal(t, testDefHash, got.DefinitionHash)
}

// TestParsePluginToken_AcceptsMultiAudience verifies that audience matching is
// set-membership: a token listing both fundament-user and fundament-plugin in
// its aud claim is accepted by ParsePluginToken regardless of element order.
func TestParsePluginToken_AcceptsMultiAudience(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.Audience = jwt.ClaimStrings{TokenTypeUser, TokenTypePlugin}
	tokenStr := signPluginToken(t, secret, c)

	_, err := ParsePluginToken(tokenStr, secret)
	require.NoError(t, err, "ParsePluginToken")
}

func TestParsePluginToken_RejectsUserAudience(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.Audience = jwt.ClaimStrings{TokenTypeUser}
	tokenStr := signPluginToken(t, secret, c)

	_, err := ParsePluginToken(tokenStr, secret)
	require.Error(t, err, "expected error for fundament-user audience")
}

func TestParsePluginToken_RejectsExpiredToken(t *testing.T) {
	secret := []byte("test-secret")
	c := validPluginClaims()
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	tokenStr := signPluginToken(t, secret, c)

	_, err := ParsePluginToken(tokenStr, secret)
	require.Error(t, err, "expected error for expired token")
}

func TestParsePluginToken_RejectsWrongSecret(t *testing.T) {
	tokenStr := signPluginToken(t, []byte("secret-a"), validPluginClaims())
	_, err := ParsePluginToken(tokenStr, []byte("secret-b"))
	require.Error(t, err, "expected error for wrong signing secret")
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
