package authn

import (
	"net/http"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
)

// HandleRefresh refreshes the JWT token.
func (s *AuthnServer) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	claims, err := s.validator.Validate(r.Header)
	if err != nil {
		s.writeErrorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	organizationIDs, err := s.getUserOrganizationIDs(r.Context(), claims.UserID)
	if err != nil {
		s.logger.Error("failed to get user organizations", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to get user organizations")
		return
	}

	u := &user{
		ID:              claims.UserID,
		OrganizationIDs: organizationIDs,
		Name:            claims.Name,
	}

	accessToken, err := s.generateJWT(u, claims.Groups)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	if err := s.writeJSON(w, http.StatusOK, authnhttp.RefreshResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.config.TokenExpiry.Seconds()),
	}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}
