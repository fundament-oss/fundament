package auth

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatorWithCookieOrigins(t *testing.T) {
	token := signToken(t, validUserClaims(uuid.NewString()))

	tests := []struct {
		name     string
		allowed  []string
		origin   string
		fetch    string // Sec-Fetch-Site
		bearer   bool
		accepted bool
	}{
		{name: "allowed origin", allowed: []string{"https://console.example.com"}, origin: "https://console.example.com", accepted: true},
		{name: "configured spelling differs", allowed: []string{" HTTPS://Console.Example.com:443/ "}, origin: "https://console.example.com", accepted: true},
		{name: "other same-site origin", allowed: []string{"https://console.example.com"}, origin: "https://plugin-proxy.example.com"},
		{name: "opaque origin", allowed: []string{"https://console.example.com"}, origin: "null"},
		{name: "same-origin fetch without Origin", allowed: []string{"https://console.example.com"}, fetch: "same-origin", accepted: true},
		{name: "same-site navigation without Origin", allowed: []string{"https://console.example.com"}, fetch: "same-site"},
		{name: "no Origin and no Sec-Fetch-Site", allowed: []string{"https://console.example.com"}},
		{name: "empty allow-list", origin: "https://console.example.com"},
		{name: "empty allow-list on a same-origin fetch", fetch: "same-origin"},
		{name: "invalid entry is dropped", allowed: []string{"console.example.com"}, origin: "https://console.example.com"},
		{name: "bearer ignores the allow-list", origin: "https://plugin-proxy.example.com", bearer: true, accepted: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newValidator().WithCookieOrigins(tc.allowed)
			header := http.Header{}
			if tc.bearer {
				header.Set("Authorization", "Bearer "+token)
			} else {
				header.Set("Cookie", ConsoleAuthCookieName+"="+token)
			}
			if tc.origin != "" {
				header.Set("Origin", tc.origin)
			}
			if tc.fetch != "" {
				header.Set("Sec-Fetch-Site", tc.fetch)
			}

			_, err := v.Validate(header)
			if tc.accepted {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidatorWithoutCookieOrigins_AcceptsAnyOrigin(t *testing.T) {
	header := http.Header{}
	header.Set("Cookie", ConsoleAuthCookieName+"="+signToken(t, validUserClaims(uuid.NewString())))
	header.Set("Origin", "https://plugin-proxy.example.com")

	_, err := newValidator().Validate(header)
	require.NoError(t, err)
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		authz string
		token string
		ok    bool
	}{
		{authz: "Bearer abc", token: "abc", ok: true},
		{authz: "Bearer "},
		{authz: "Basic abc"},
		{authz: ""},
	}
	for _, tc := range tests {
		header := http.Header{}
		header.Set("Authorization", tc.authz)
		token, ok := BearerToken(header)
		assert.Equal(t, tc.ok, ok, tc.authz)
		assert.Equal(t, tc.token, token, tc.authz)
	}
}
