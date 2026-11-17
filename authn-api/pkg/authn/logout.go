package authn

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// idTokenCookieName holds the identity provider's ID token from the last OIDC
// login. Logout hands it back to the provider as id_token_hint, which lets the
// provider end its own session without asking the user to confirm.
const idTokenCookieName = "fundament_id_token" //nolint:gosec // a cookie name, not a credential

// idTokenCookiePath scopes the ID token cookie to the one request that needs
// it, so the browser does not send it along with every other authn call.
const idTokenCookiePath = "/logout"

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

// buildIDTokenCookie keeps the provider's ID token until logout, for as long
// as the auth cookie lives. Host-only and scoped to /logout: no other service
// on the cookie domain has any use for it.
func (s *AuthnServer) buildIDTokenCookie(idToken string) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure comes from config; HttpOnly and SameSite are set
		Name:     idTokenCookieName,
		Value:    idToken,
		Path:     idTokenCookiePath,
		MaxAge:   int(s.config.TokenExpiry.Seconds()),
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
