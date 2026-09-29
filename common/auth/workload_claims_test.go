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

const testWorkload = "plugin-controller"

func signWorkloadToken(t *testing.T, secret []byte, c *WorkloadClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	s, err := tok.SignedString(secret)
	require.NoError(t, err, "sign")
	return s
}

func validWorkloadClaims() *WorkloadClaims {
	return &WorkloadClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    ConsoleIssuer,
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{TokenTypeWorkload},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.New().String(),
		},
		OrganizationID: uuid.New().String(),
		Workload:       testWorkload,
	}
}

func TestParseWorkloadToken_AcceptsValidToken(t *testing.T) {
	secret := []byte("test-secret")
	want := validWorkloadClaims()
	tokenStr := signWorkloadToken(t, secret, want)

	got, err := ParseWorkloadToken(tokenStr, secret)
	require.NoError(t, err, "ParseWorkloadToken")
	assert.Equal(t, want.Subject, got.Subject)
	assert.Equal(t, uuid.MustParse(want.Subject), got.ClusterID())
	assert.Equal(t, want.OrganizationID, got.OrganizationID)
	assert.Equal(t, testWorkload, got.Workload)
	assert.Equal(t, want.ID, got.ID)
}

func TestParseWorkloadToken_Rejects(t *testing.T) {
	secret := []byte("test-secret")
	tests := []struct {
		name   string
		mutate func(c *WorkloadClaims)
	}{
		{"user audience", func(c *WorkloadClaims) { c.Audience = jwt.ClaimStrings{TokenTypeUser} }},
		{"plugin audience", func(c *WorkloadClaims) { c.Audience = jwt.ClaimStrings{TokenTypePlugin} }},
		{"no audience", func(c *WorkloadClaims) { c.Audience = nil }},
		{"wrong issuer", func(c *WorkloadClaims) { c.Issuer = DCIMIssuer }},
		{"expired", func(c *WorkloadClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour)) }},
		{"missing exp", func(c *WorkloadClaims) { c.ExpiresAt = nil }},
		{"non-UUID subject", func(c *WorkloadClaims) { c.Subject = "not-a-uuid" }},
		{"non-UUID organization", func(c *WorkloadClaims) { c.OrganizationID = "acme" }},
		{"empty organization", func(c *WorkloadClaims) { c.OrganizationID = "" }},
		{"empty workload", func(c *WorkloadClaims) { c.Workload = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validWorkloadClaims()
			tt.mutate(c)
			_, err := ParseWorkloadToken(signWorkloadToken(t, secret, c), secret)
			require.Error(t, err)
		})
	}
}

func TestParseWorkloadToken_RejectsWrongSecret(t *testing.T) {
	tokenStr := signWorkloadToken(t, []byte("secret-a"), validWorkloadClaims())
	_, err := ParseWorkloadToken(tokenStr, []byte("secret-b"))
	require.Error(t, err)
}

func TestParseWorkloadToken_RejectsNonHS256Method(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-hs384-and-hs512")
	tok := jwt.NewWithClaims(jwt.SigningMethodHS384, validWorkloadClaims())
	tokenStr, err := tok.SignedString(secret)
	require.NoError(t, err)

	_, err = ParseWorkloadToken(tokenStr, secret)
	require.Error(t, err)
}

// TestWorkloadToken_AudienceWall pins the FUN-17/FUN-22 wall from the
// library side: a WorkloadToken (same secret, same issuer) is rejected by the
// user validator and by ParsePluginToken, and a UserToken or PluginToken is
// rejected by ParseWorkloadToken.
func TestWorkloadToken_AudienceWall(t *testing.T) {
	secret := []byte("test-secret")
	workloadTok := signWorkloadToken(t, secret, validWorkloadClaims())

	userValidator := NewValidatorForAudience(secret, ConsoleAuthCookieName, ConsoleIssuer, TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+workloadTok)
	_, err := userValidator.Validate(header)
	require.Error(t, err, "user validator accepted a WorkloadToken")
	assert.Contains(t, err.Error(), "audience")

	_, err = ParsePluginToken(workloadTok, secret)
	require.Error(t, err, "ParsePluginToken accepted a WorkloadToken")
	assert.Contains(t, err.Error(), "audience")

	// The DCIM validator has no audience pin; it must fall on the issuer.
	dcimValidator := NewValidator(secret, DCIMAuthCookieName, DCIMIssuer, nil)
	_, err = dcimValidator.Validate(header)
	require.Error(t, err, "DCIM validator accepted a WorkloadToken")

	pluginTok := signPluginToken(t, secret, validPluginClaims())
	_, err = ParseWorkloadToken(pluginTok, secret)
	require.Error(t, err, "ParseWorkloadToken accepted a PluginToken")

	userClaims := &Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    ConsoleIssuer,
		Subject:   uuid.New().String(),
		Audience:  jwt.ClaimStrings{TokenTypeUser},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	userTok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, userClaims).SignedString(secret)
	require.NoError(t, err)
	_, err = ParseWorkloadToken(userTok, secret)
	require.Error(t, err, "ParseWorkloadToken accepted a UserToken")
}
