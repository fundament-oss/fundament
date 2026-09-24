package auth

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testReturnOrigins(allowed ...string) ReturnOrigins {
	return NewReturnOrigins(slog.New(slog.NewTextHandler(io.Discard, nil)), allowed)
}

func TestReturnOriginsAllows(t *testing.T) {
	origins := testReturnOrigins(
		"https://console.fundament.localhost:8443",
		"https://marketplace-registry.fundament.localhost:8443",
		"http://localhost:4200",
	)

	tests := []struct {
		name     string
		returnTo string
		want     bool
	}{
		{"listed origin with a path", "https://marketplace-registry.fundament.localhost:8443/manage", true},
		{"listed origin bare", "https://console.fundament.localhost:8443", true},
		{"listed origin with query and fragment", "http://localhost:4200/manage?tab=drafts#top", true},
		{"scheme is compared", "http://marketplace-registry.fundament.localhost:8443/manage", false},
		{"port is compared", "https://marketplace-registry.fundament.localhost:9443/manage", false},
		{"host is compared", "https://evil.example/manage", false},
		{"listed origin as a host suffix", "https://evil-marketplace-registry.fundament.localhost:8443/", false},
		{"listed origin as a subdomain of elsewhere", "https://console.fundament.localhost.evil.example/", false},
		{"userinfo naming a listed origin", "https://console.fundament.localhost:8443@evil.example/", false},
		{"protocol-relative", "//evil.example/manage", false},
		{"relative path", "/manage", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"data scheme", "data:text/html,<script>alert(1)</script>", false},
		{"empty", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, origins.Allows(test.returnTo))
		})
	}
}

// A default port and no port name the same origin, so a return URL that spells
// one out still matches a configured origin that does not, and the reverse.
func TestReturnOriginsAllows_DefaultPorts(t *testing.T) {
	origins := testReturnOrigins(
		"https://console.example.com",
		"http://portal.example.com:80",
	)

	assert.True(t, origins.Allows("https://console.example.com:443/manage"))
	assert.True(t, origins.Allows("http://portal.example.com/manage"))
	assert.False(t, origins.Allows("https://console.example.com:8443/manage"))
}

// Case is not significant in a scheme or a host, but it is in a path, so a
// listed origin still matches when the caller shouts it.
func TestReturnOriginsAllows_CaseInsensitiveOrigin(t *testing.T) {
	origins := testReturnOrigins("https://marketplace-registry.fundament.localhost:8443")

	assert.True(t, origins.Allows("HTTPS://Marketplace-Registry.Fundament.Localhost:8443/manage"))
}

// A configured origin that cannot be parsed can never match, so it is dropped
// at startup and said out loud there — the login it refuses only logs the
// caller's return_to, which points at the wrong thing.
func TestNewReturnOrigins_ReportsUnusableEntries(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	origins := NewReturnOrigins(logger, []string{
		"https://console.example.com:443",
		"",
		"console.example.com",
		"HTTPS://Console.Example.com",
	})

	// The scheme-less entry is gone, the empty one is the trailing comma of a
	// comma-separated variable, and the two spellings of the console fold into
	// one origin.
	assert.Equal(t, ReturnOrigins{"https://console.example.com"}, origins)
	assert.Contains(t, logs.String(), "console.example.com")
	assert.Contains(t, logs.String(), "not an absolute http(s) URL")
}

func TestNewReturnOrigins_ReportsAnEmptyAllowlist(t *testing.T) {
	var logs bytes.Buffer

	origins := NewReturnOrigins(slog.New(slog.NewTextHandler(&logs, nil)), []string{"", "nope"})

	assert.Empty(t, origins)
	assert.Contains(t, logs.String(), "no usable allowed return origins")
}
