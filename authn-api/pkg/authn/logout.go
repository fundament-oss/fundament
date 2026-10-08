package authn

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
)

// HandleLogout revokes the browser's session and clears both auth cookies.
//
// Revoking is what makes the access token already in the wild stop working,
// within its own lifetime: nothing can mint another one from a revoked session
// (FUN-23). A logout that arrives without a refresh cookie still clears cookies
// and still answers ok — a caller who no longer holds the credential has
// nothing to prove and nothing to revoke, and answering differently would tell
// an attacker which cookies they hold (FUN-12).
func (s *AuthnServer) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if token := refreshTokenFromRequest(r); token != "" {
		s.revokeSessionForToken(r, token)
	}

	http.SetCookie(w, s.buildClearAuthCookie())
	http.SetCookie(w, s.buildClearRefreshCookie())

	s.logger.Debug("user logged out")

	if err := s.writeJSON(w, http.StatusOK, authnhttp.StatusResponse{Status: new("ok")}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

// revokeSessionForToken ends the session a logout's refresh token belongs to.
// Failure is logged rather than answered: the cookies are cleared either way,
// and a logout that reported an error because the database was briefly
// unreachable would leave the user looking signed in.
func (s *AuthnServer) revokeSessionForToken(r *http.Request, token string) {
	row, err := s.webSessions.WebSessionGetByHash(r.Context(), db.WebSessionGetByHashParams{
		TokenHash: hashRefreshToken(token),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Info("logout presented an unknown refresh token")
			return
		}
		s.logger.Error("failed to look up session for logout", "error", err)
		return
	}

	s.revokeWebSession(r.Context(), row.SessionID, "logout")
}
