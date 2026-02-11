package authn

import (
	"net/http"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// HandleLogout clears the auth cookie.
func (s *AuthnServer) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("user logged out")

	http.SetCookie(w, s.buildClearAuthCookie())
	if err := s.writeJSON(w, http.StatusOK, authnhttp.StatusResponse{Status: new("ok")}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}
