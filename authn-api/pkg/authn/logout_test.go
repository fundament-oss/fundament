package authn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	"github.com/fundament-oss/fundament/common/auth"
)

const testEndSessionURL = "https://login.test-fundamentsso.nl/realms/fundamentsso/protocol/openid-connect/logout"

// idTokenCookie is the cookie as the browser sends it back: name and value
// only, the attributes stay with the browser.
func idTokenCookie() *http.Cookie {
	return &http.Cookie{Name: idTokenCookieName, Value: "the-id-token"} //nolint:gosec // a request cookie carries no attributes
}

// logoutServer builds a server that can run the logout handler without a
// database, with the end-session endpoint the provider advertises (or none).
func logoutServer(endSessionURL string) *AuthnServer {
	server := loginServer()
	server.config.EndSessionURL = endSessionURL
	server.config.TokenExpiry = time.Hour
	server.cookieBuilder = auth.NewCookieBuilder("fundament.localhost", true, auth.ConsoleAuthCookieName)
	return server
}

func logout(t *testing.T, server *AuthnServer, cookies ...*http.Cookie) (*httptest.ResponseRecorder, authnhttp.LogoutResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/logout", http.NoBody)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	server.HandleLogout(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body authnhttp.LogoutResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&body))
	return recorder, body
}

func TestHandleLogout_ClearsBothCookies(t *testing.T) {
	recorder, _ := logout(t, logoutServer(testEndSessionURL))

	cleared := map[string]*http.Cookie{}
	for _, cookie := range recorder.Result().Cookies() {
		cleared[cookie.Name] = cookie
	}
	require.Contains(t, cleared, auth.ConsoleAuthCookieName)
	require.Contains(t, cleared, idTokenCookieName)
	assert.Equal(t, -1, cleared[auth.ConsoleAuthCookieName].MaxAge)
	assert.Equal(t, -1, cleared[idTokenCookieName].MaxAge)
	assert.Equal(t, idTokenCookiePath, cleared[idTokenCookieName].Path)
}

func TestHandleLogout_WithoutEndSessionEndpoint(t *testing.T) {
	_, body := logout(t, logoutServer(""), idTokenCookie())

	assert.Equal(t, "ok", *body.Status)
	assert.Nil(t, body.LogoutUrl, "Dex has no end-session endpoint, so there is nowhere to send the browser")
}

func TestHandleLogout_EndSessionURL(t *testing.T) {
	tests := map[string]struct {
		cookies  []*http.Cookie
		wantHint string
	}{
		"with the ID token":    {cookies: []*http.Cookie{idTokenCookie()}, wantHint: "the-id-token"},
		"without the ID token": {},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, body := logout(t, logoutServer(testEndSessionURL), tt.cookies...)

			require.NotNil(t, body.LogoutUrl)
			logoutURL, err := url.Parse(*body.LogoutUrl)
			require.NoError(t, err)
			assert.Equal(t, testEndSessionURL, logoutURL.Scheme+"://"+logoutURL.Host+logoutURL.Path)
			assert.Equal(t, "fundament-poc", logoutURL.Query().Get("client_id"))
			assert.Equal(t, "https://console.fundament.localhost:8443/login", logoutURL.Query().Get("post_logout_redirect_uri"))
			if tt.wantHint == "" {
				assert.NotContains(t, logoutURL.Query(), "id_token_hint")
			} else {
				assert.Equal(t, tt.wantHint, logoutURL.Query().Get("id_token_hint"))
			}
		})
	}
}

func TestSetIDTokenCookie(t *testing.T) {
	// Name and value fill the limit exactly; the attributes push it over.
	fitsWithoutAttributes := strings.Repeat("x", maxIDTokenCookieBytes-len(idTokenCookieName))

	tests := map[string]struct {
		endSessionURL string
		idToken       string
		// "kept", "none", or "cleared": an earlier login's token is removed,
		// so logout does not hand the provider a hint for another session.
		want string
	}{
		"with an end-session endpoint":        {endSessionURL: testEndSessionURL, idToken: "the-id-token", want: "kept"},
		"without one (Dex)":                   {idToken: "the-id-token", want: "none"},
		"too large for the browser":           {endSessionURL: testEndSessionURL, idToken: strings.Repeat("x", maxIDTokenCookieBytes), want: "cleared"},
		"too large once its attributes count": {endSessionURL: testEndSessionURL, idToken: fitsWithoutAttributes, want: "cleared"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			logoutServer(tt.endSessionURL).setIDTokenCookie(recorder, tt.idToken)

			cookies := recorder.Result().Cookies()
			switch tt.want {
			case "none":
				assert.Empty(t, cookies)
			case "kept":
				require.Len(t, cookies, 1)
				assert.Equal(t, tt.idToken, cookies[0].Value)
			case "cleared":
				require.Len(t, cookies, 1)
				assert.Equal(t, idTokenCookieName, cookies[0].Name)
				assert.Empty(t, cookies[0].Value)
				assert.Equal(t, -1, cookies[0].MaxAge)
			default:
				panic("unknown want: " + tt.want)
			}
		})
	}
}

func TestBuildIDTokenCookie(t *testing.T) {
	cookie := logoutServer(testEndSessionURL).buildIDTokenCookie("the-id-token")

	assert.Equal(t, idTokenCookieName, cookie.Name)
	assert.Equal(t, "the-id-token", cookie.Value)
	assert.Equal(t, idTokenCookiePath, cookie.Path)
	assert.Empty(t, cookie.Domain, "only authn reads it, so it stays on authn's host")
	// Outlives the token: /refresh extends the session past TokenExpiry.
	assert.Equal(t, int(idTokenCookieMaxAge.Seconds()), cookie.MaxAge)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
}
