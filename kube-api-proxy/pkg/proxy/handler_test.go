package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
)

// signWith produces an HS256-signed token with the given audiences. Non-test
// callers should never mint tokens this way; peekTokenType only inspects the
// audience without verifying the signature, so the exact bytes are what matter
// for this test.
func signWith(t *testing.T, aud jwt.ClaimStrings) string {
	t.Helper()
	c := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			Issuer:    auth.ConsoleIssuer,
			Audience:  aud,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte("ignored"))
	require.NoError(t, err)
	return s
}

func TestPeekTokenType(t *testing.T) {
	pluginTok := signWith(t, jwt.ClaimStrings{auth.TokenTypePlugin})
	userTok := signWith(t, jwt.ClaimStrings{auth.TokenTypeUser})
	mixedTok := signWith(t, jwt.ClaimStrings{auth.TokenTypeUser, auth.TokenTypePlugin})

	cases := []struct {
		name       string
		authHeader string
		cookie     *http.Cookie
		want       auth.TokenType
	}{
		{"no auth", "", nil, ""},
		{"malformed bearer", "Bearer not.a.jwt", nil, ""},
		{"plugin bearer", "Bearer " + pluginTok, nil, auth.TokenTypePlugin},
		{"user bearer", "Bearer " + userTok, nil, auth.TokenTypeUser},
		{"mixed audience is plugin", "Bearer " + mixedTok, nil, auth.TokenTypePlugin},
		{"cookie only", "", &http.Cookie{Name: auth.ConsoleAuthCookieName, Value: "irrelevant"}, auth.TokenTypeUser},
		{"empty cookie value", "", &http.Cookie{Name: auth.ConsoleAuthCookieName, Value: ""}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/pods", http.NoBody)
			if tc.authHeader != "" {
				r.Header.Set("Authorization", tc.authHeader)
			}
			if tc.cookie != nil {
				r.AddCookie(tc.cookie)
			}
			assert.Equal(t, tc.want, peekTokenType(r))
		})
	}
}

// TestIsAllowedPath feeds raw {path...} wildcard values as r.PathValue returns
// them in production (percent-decoded, no leading slash) and checks whole-segment
// matching of the first path segment.
func TestIsAllowedPath(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"api", true},
		{"api/v1/pods", true},
		{"apis", true},
		{"apis/apps/v1/deployments", true},
		{"openapi/v3", true},
		{"version", true},
		{"", false},
		{"healthz", false},
		{"livez", false},
		{"metrics", false},
		{"logs", false},
		// Prefix collisions must not match: only whole path segments count.
		{"apix", false},
		{"apisx/apps", false},
		{"versionz", false},
		{"openapix/v3", false},
	}
	for _, tc := range cases {
		t.Run("path "+tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, isAllowedPath(tc.raw))
		})
	}
}
