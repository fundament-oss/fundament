package authn

import (
	"net/http"

	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// HandleLogin initiates the OIDC login flow by redirecting to the provider.
func (s *AuthnServer) HandleLogin(w http.ResponseWriter, r *http.Request, params authnhttp.HandleLoginParams) {
	var returnTo string
	if params.ReturnTo != nil && *params.ReturnTo != "" {
		// Refused rather than quietly replaced by the frontend URL: a caller
		// that names an origin this deployment does not serve is either an
		// open-redirect attempt or a misconfigured surface, and neither should
		// look like a successful login to somewhere else.
		if !s.isSafeReturnTo(*params.ReturnTo) {
			s.logger.Warn("rejected return_to outside the allowlist", "return_to", *params.ReturnTo)
			s.writeErrorJSON(w, http.StatusBadRequest, "return_to is not an allowed origin")
			return
		}
		returnTo = *params.ReturnTo
	}

	state, err := generateState(returnTo)
	if err != nil {
		s.logger.Error("failed to generate state", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	verifier := oauth2.GenerateVerifier()

	if err := s.sessionStore.SetOAuthParams(w, r, state, verifier); err != nil {
		s.logger.Error("failed to store OAuth params in session", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	authURL := s.oauth2Config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	s.logger.Debug("redirecting to OIDC provider", "url", authURL, "return_to", returnTo)

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}
