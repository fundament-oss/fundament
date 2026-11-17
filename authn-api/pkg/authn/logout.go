package authn

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// idTokenCookieName holds the identity provider's ID token from the last OIDC
// login. Logout hands it back to the provider as id_token_hint, which lets the
// provider end its own session without asking the user to confirm.
const idTokenCookieName = "fundament_id_token" //nolint:gosec // a cookie name, not a credential

// idTokenCookiePath scopes the ID token cookie to the one request that needs
// it, so the browser does not send it along with every other authn call.
const idTokenCookiePath = "/logout"

// idTokenCookieMaxAge is how long the ID token cookie lives. Not TokenExpiry:
// /refresh keeps a session going past it, and cannot renew this cookie (the
// browser sends it to /logout only). An expired ID token is still a valid
// hint: OpenID Connect RP-Initiated Logout 1.0 asks the provider to accept
// one past its exp. Logout clears the cookie, and the next login replaces it.
const idTokenCookieMaxAge = 30 * 24 * time.Hour

// maxIDTokenCookieBytes is the most a browser keeps of a cookie's name and
// value; a larger one is dropped without a word.
const maxIDTokenCookieBytes = 4096

// HandleLogout clears the auth cookie. When the identity provider supports
// RP-initiated logout, it also returns the URL that ends the session there:
// clearing only Fundament's cookie leaves the provider's single sign-on
// session in place, and the next login would go straight through.
func (s *AuthnServer) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("user logged out")

	response := authnhttp.LogoutResponse{Status: new("ok")}
	if logoutURL := s.providerLogoutURL(r); logoutURL != "" {
		response.LogoutUrl = &logoutURL
	}

	http.SetCookie(w, s.buildClearAuthCookie())
	http.SetCookie(w, s.buildClearIDTokenCookie())
	if err := s.writeJSON(w, http.StatusOK, response); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

// providerLogoutURL builds the identity provider's end-session URL, or returns
// "" when the provider has none (Dex). Without an ID token, from a session
// that started before the cookie existed, the provider still logs out but
// asks the user to confirm first.
func (s *AuthnServer) providerLogoutURL(r *http.Request) string {
	if s.config.EndSessionURL == "" {
		return ""
	}
	endSession, err := url.Parse(s.config.EndSessionURL)
	if err != nil {
		s.logger.Error("invalid end session URL", "error", err)
		return ""
	}

	query := endSession.Query()
	query.Set("client_id", s.oauth2Config.ClientID)
	query.Set("post_logout_redirect_uri", strings.TrimSuffix(s.config.FrontendURL, "/")+"/login")
	if cookie, err := r.Cookie(idTokenCookieName); err == nil && cookie.Value != "" {
		query.Set("id_token_hint", cookie.Value)
	}
	endSession.RawQuery = query.Encode()
	return endSession.String()
}

// setIDTokenCookie keeps the provider's ID token for logout, after a login
// through the provider (the code flow or the password grant). Only where
// logout can use it: a provider without an end-session endpoint has nothing
// to hand it back to.
func (s *AuthnServer) setIDTokenCookie(w http.ResponseWriter, idToken string) {
	if s.config.EndSessionURL == "" {
		return
	}
	if len(idTokenCookieName)+len(idToken) > maxIDTokenCookieBytes {
		// The browser would drop it anyway. Logout still works, only with the
		// provider asking the user to confirm.
		s.logger.Warn("ID token too large to keep for logout", "bytes", len(idToken))
		return
	}
	http.SetCookie(w, s.buildIDTokenCookie(idToken))
}

// buildIDTokenCookie keeps the provider's ID token until logout (see
// idTokenCookieMaxAge). Host-only and scoped to /logout: no other service on
// the cookie domain has any use for it.
func (s *AuthnServer) buildIDTokenCookie(idToken string) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure comes from config; HttpOnly and SameSite are set
		Name:     idTokenCookieName,
		Value:    idToken,
		Path:     idTokenCookiePath,
		MaxAge:   int(idTokenCookieMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

// buildClearIDTokenCookie returns a cookie that clears the ID token cookie.
func (s *AuthnServer) buildClearIDTokenCookie() *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure comes from config; HttpOnly and SameSite are set
		Name:     idTokenCookieName,
		Value:    "",
		Path:     idTokenCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}
