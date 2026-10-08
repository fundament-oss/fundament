package authn

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
)

// HandleRefresh mints the next access token.
//
// A browser presents the refresh cookie and gets a rotated session back
// (FUN-23). A browser that signed in before web sessions existed has no such
// cookie, and falls back to the pre-FUN-23 behaviour of re-minting from the
// access token it presents. The fallback is temporary: it keeps sessions alive
// across the deploy that introduces sessions, and goes away with the step that
// shortens the access token, at which point a refresh without the cookie is
// simply unauthenticated.
func (s *AuthnServer) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if token := refreshTokenFromRequest(r); token != "" {
		s.refreshFromSession(w, r, token)
		return
	}

	s.refreshFromAccessToken(w, r)
}

// refreshFromSession spends a refresh token and answers with the access token
// minted from its successor.
func (s *AuthnServer) refreshFromSession(w http.ResponseWriter, r *http.Request, token string) {
	session, err := s.spendRefreshToken(r.Context(), token)
	if err != nil {
		// One answer for every reason a session is not good: an unknown token,
		// an expired one, a revoked one and a replayed one are the same 401 to
		// the caller, with the reason in the log (FUN-12). The browser is also
		// sent the clearing cookies, so a dead session stops being presented
		// on every subsequent request.
		s.logger.Info("refresh refused", "error", err)
		http.SetCookie(w, s.buildClearAuthCookie())
		http.SetCookie(w, s.buildClearRefreshCookie())
		s.writeErrorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	u, err := s.sessionUser(r.Context(), session)
	if err != nil {
		s.logger.Error("failed to assemble the refreshed token's subject", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to refresh token")
		return
	}

	accessToken, err := s.generateJWT(u, session.Groups, session.SessionID.String())
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	http.SetCookie(w, s.buildRefreshCookie(session.RefreshToken))
	s.writeRefreshResponse(w, accessToken)
}

// refreshFromAccessToken is the pre-FUN-23 path: re-mint from a still-valid
// access token, for a browser that signed in before sessions existed.
func (s *AuthnServer) refreshFromAccessToken(w http.ResponseWriter, r *http.Request) {
	claims, err := s.validator.Validate(r.Header)
	if err != nil {
		s.writeErrorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	userID := claims.UserID()

	organizationIDs, err := s.getUserOrganizationIDs(r.Context(), userID)
	if err != nil {
		s.logger.Error("failed to get user organizations", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to get user organizations")
		return
	}

	u := &user{
		ID:              userID,
		OrganizationIDs: organizationIDs,
		Name:            claims.Name,
	}

	accessToken, err := s.generateJWT(u, claims.Groups, claims.SessionID)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	s.logger.Debug("refreshed a session without a refresh cookie", "user_id", userID)

	http.SetCookie(w, s.buildAuthCookie(accessToken))
	s.writeRefreshResponse(w, accessToken)
}

// sessionUser assembles the token's subject from the session row plus the user
// and memberships as they are now.
//
// Both are read fresh on every refresh. Memberships, because that is how one
// assigned by an operator reaches a user who is already signed in. The name,
// because GetUserInfo answers with the `name` claim rather than with a query,
// so a token minted without it would blank the name the console displays —
// which is why a failure here is an error rather than an empty string. Groups
// are the exception: they come from the identity provider's ID token, and a
// refresh has none, so they ride on the session row (FUN-23).
func (s *AuthnServer) sessionUser(ctx context.Context, session *webSession) (*user, error) {
	row, err := s.queries.UserGetByID(ctx, db.UserGetByIDParams{ID: session.UserID})
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}

	organizationIDs, err := s.getUserOrganizationIDs(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	return &user{
		ID:              session.UserID,
		OrganizationIDs: organizationIDs,
		Name:            row.Name,
		ExternalRef:     row.ExternalRef.String,
	}, nil
}

func (s *AuthnServer) writeRefreshResponse(w http.ResponseWriter, accessToken string) {
	if err := s.writeJSON(w, http.StatusOK, authnhttp.RefreshResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.config.TokenExpiry.Seconds()),
	}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}
