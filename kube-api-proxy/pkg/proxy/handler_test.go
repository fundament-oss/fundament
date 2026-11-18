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

func signWith(t *testing.T, secret []byte, aud jwt.ClaimStrings) string {
	t.Helper()
	c := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			Issuer:    auth.ConsoleIssuer,
			Audience:  aud,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	require.NoError(t, err)
	return s
}

func TestIsPluginToken(t *testing.T) {
	secret := []byte("secret")

	cases := []struct {
		name       string
		authHeader string
		cookie     *http.Cookie
		want       bool
	}{
		{"no auth", "", nil, false},
		{"malformed bearer", "Bearer not.a.jwt", nil, false},
		{"plugin bearer", "Bearer " + signWith(t, secret, jwt.ClaimStrings{auth.TokenTypePlugin}), nil, true},
		{"plugin audience with a forged signature", "Bearer " + signWith(t, []byte("other"), jwt.ClaimStrings{auth.TokenTypePlugin}), nil, false},
		{"user bearer", "Bearer " + signWith(t, secret, jwt.ClaimStrings{auth.TokenTypeUser}), nil, false},
		{"mixed audience is plugin", "Bearer " + signWith(t, secret, jwt.ClaimStrings{auth.TokenTypeUser, auth.TokenTypePlugin}), nil, true},
		{"cookie only", "", &http.Cookie{Name: auth.ConsoleAuthCookieName, Value: signWith(t, secret, jwt.ClaimStrings{auth.TokenTypePlugin})}, false}, //nolint:gosec // request cookie in a test
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
			assert.Equal(t, tc.want, isPluginToken(r, secret))
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
		{"healthz", true},
		{"livez", true},
		{"readyz/ping", true},
		{"", false},
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

// The proxy answers namespace listings and access reviews for the caller; an
// impersonated request (kubectl --as) goes to the apiserver instead.
func TestServesNamespaceListAndAccessReview(t *testing.T) {
	t.Parallel()
	list := func(impersonate bool) *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces", http.NoBody)
		if impersonate {
			r.Header.Set("Impersonate-User", "bob")
		}
		return r
	}
	review := func(impersonate bool) *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, accessReviewPath, http.NoBody)
		if impersonate {
			r.Header["impersonate-group"] = []string{"devs"} // non-canonical key
		}
		return r
	}

	assert.True(t, servesNamespaceList(list(false)))
	assert.False(t, servesNamespaceList(list(true)))
	assert.True(t, servesAccessReview(review(false)))
	assert.False(t, servesAccessReview(review(true)))
	assert.False(t, servesNamespaceList(review(false)))
	assert.False(t, servesAccessReview(list(false)))
}
