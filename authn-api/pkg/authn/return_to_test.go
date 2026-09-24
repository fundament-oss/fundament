package authn

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	"github.com/fundament-oss/fundament/common/auth"
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
		allowedReturnOrigins: auth.NewReturnOrigins(logger, allowed),
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

// The matching itself is tested with auth.ReturnOrigins; this only checks
// that the server consults the list it was built with.
func TestReturnToAllowed(t *testing.T) {
	server := allowlistServer()

	assert.True(t, server.isSafeReturnTo("https://marketplace-registry.fundament.localhost:8443/manage"))
	assert.False(t, server.isSafeReturnTo("https://evil.example/manage"))
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
