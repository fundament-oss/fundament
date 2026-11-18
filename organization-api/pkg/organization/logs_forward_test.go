package organization

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/common/auth"
)

func TestForwardedAuthHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{name: "bearer", header: http.Header{"Authorization": {"Bearer tok"}}, want: "Bearer tok"},
		{name: "session cookie", header: http.Header{"Cookie": {"other=x; " + auth.ConsoleAuthCookieName + "=tok"}}, want: "Bearer tok"},
		{name: "bearer wins over the cookie", header: http.Header{"Authorization": {"Bearer tok"}, "Cookie": {auth.ConsoleAuthCookieName + "=other"}}, want: "Bearer tok"},
		{name: "no credentials", header: http.Header{"Cookie": {"other=x"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := forwardedAuthHeaders(tc.header)
			assert.Equal(t, tc.want, got.Get("Authorization"))
			assert.Empty(t, got.Values("Cookie"), "the cookie is never forwarded")
		})
	}
}
