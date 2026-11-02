package authn

import (
	"net/http"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// HandleLoginMethods tells the login page how this deployment signs people in:
// with the password form (Dex's static users), with a button that starts
// /login at the identity provider, or both.
func (s *AuthnServer) HandleLoginMethods(w http.ResponseWriter, _ *http.Request) {
	methods := authnhttp.LoginMethodsResponse{Password: s.config.PasswordLoginEnabled}
	if s.config.OIDCDisplayName != "" {
		methods.ProviderName = &s.config.OIDCDisplayName
	}

	if err := s.writeJSON(w, http.StatusOK, methods); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}
