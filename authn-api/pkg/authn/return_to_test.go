package authn

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// serverWithAllowlist builds the server the way New does, so the tests run
// against the same normalized allowlist production does.
func serverWithAllowlist(frontendURL string, allowed ...string) *AuthnServer {
	logger := discardLogger()

	return &AuthnServer{
		logger:               logger,
		config:               &Config{FrontendURL: frontendURL, AllowedReturnOrigins: allowed},
		allowedReturnOrigins: normalizeReturnOrigins(logger, allowed),
	}
}

func allowlistServer() *AuthnServer {
	return serverWithAllowlist(
		"https://console.fundament.localhost:8443",
		"https://console.fundament.localhost:8443",
		"https://marketplace-registry.fundament.localhost:8443",
		"http://localhost:4200",
	)
}

func TestReturnToAllowed(t *testing.T) {
	server := allowlistServer()

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
			assert.Equal(t, test.want, server.isSafeReturnTo(test.returnTo))
		})
	}
}

// A default port and no port name the same origin, so a return URL that spells
// one out still matches a configured origin that does not, and the reverse.
func TestReturnToAllowed_DefaultPorts(t *testing.T) {
	server := serverWithAllowlist("",
		"https://console.example.com",
		"http://portal.example.com:80",
	)

	assert.True(t, server.isSafeReturnTo("https://console.example.com:443/manage"))
	assert.True(t, server.isSafeReturnTo("http://portal.example.com/manage"))
	assert.False(t, server.isSafeReturnTo("https://console.example.com:8443/manage"))
}

// Case is not significant in a scheme or a host, but it is in a path, so a
// listed origin still matches when the caller shouts it.
func TestReturnToAllowed_CaseInsensitiveOrigin(t *testing.T) {
	server := allowlistServer()

	assert.True(t, server.isSafeReturnTo("HTTPS://Marketplace-Registry.Fundament.Localhost:8443/manage"))
}

func TestHandleLogin_RejectsReturnToOutsideAllowlist(t *testing.T) {
	server := allowlistServer()

	returnTo := "https://evil.example/steal"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/login?return_to="+returnTo, http.NoBody)

	server.HandleLogin(recorder, request, authnhttp.HandleLoginParams{ReturnTo: &returnTo})

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Location"))
}

// The state is a bearer of the return URL, so the callback checks it too. A
// value that is no longer allowed falls back to the frontend URL rather than
// erroring: the visitor has authenticated by then.
func TestGetRedirectURL(t *testing.T) {
	server := allowlistServer()

	allowedState, err := generateState("https://marketplace-registry.fundament.localhost:8443/manage")
	require.NoError(t, err)
	assert.Equal(t,
		"https://marketplace-registry.fundament.localhost:8443/manage",
		server.getRedirectURL(allowedState))

	deniedState, err := generateState("https://evil.example/steal")
	require.NoError(t, err)
	assert.Equal(t, server.config.FrontendURL, server.getRedirectURL(deniedState))

	emptyState, err := generateState("")
	require.NoError(t, err)
	assert.Equal(t, server.config.FrontendURL, server.getRedirectURL(emptyState))

	assert.Equal(t, server.config.FrontendURL, server.getRedirectURL("not-a-state"))
}

// A configured origin that cannot be parsed can never match, so it is dropped
// at startup and said out loud there — the login it refuses only logs the
// caller's return_to, which points at the wrong thing.
func TestNormalizeReturnOrigins_ReportsUnusableEntries(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	origins := normalizeReturnOrigins(logger, []string{
		"https://console.example.com:443",
		"",
		"console.example.com",
		"HTTPS://Console.Example.com",
	})

	// The scheme-less entry is gone, the empty one is the trailing comma of a
	// comma-separated variable, and the two spellings of the console fold into
	// one origin.
	assert.Equal(t, []string{"https://console.example.com"}, origins)
	assert.Contains(t, logs.String(), "console.example.com")
	assert.Contains(t, logs.String(), "not an absolute http(s) URL")
}

func TestNormalizeReturnOrigins_ReportsAnEmptyAllowlist(t *testing.T) {
	var logs bytes.Buffer

	origins := normalizeReturnOrigins(slog.New(slog.NewTextHandler(&logs, nil)), []string{"", "nope"})

	assert.Empty(t, origins)
	assert.Contains(t, logs.String(), "no usable allowed return origins")
}
