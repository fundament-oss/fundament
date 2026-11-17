package authn

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
)

// HandleRefresh refreshes the JWT token.
func (s *AuthnServer) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	claims, err := s.validator.Validate(r.Header)
	if err != nil {
		s.writeErrorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// A session ends SessionMaxAge after its login, however often it is
	// refreshed. API-key tokens are not sessions: the key is exchanged again.
	now := time.Now()
	if claims.AuthTime == nil || slices.Contains(claims.AMR, amrAPIKey) ||
		!now.Before(claims.AuthTime.Add(s.config.SessionMaxAge)) {
		s.writeErrorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	sess := session{authTime: claims.AuthTime.Time, amr: claims.AMR}

	// Refresh only extends a session whose subject still resolves to a live
	// account, so a soft-deleted user cannot keep a session alive. The name
	// comes from the account rather than the old token, so a rename lands on
	// the next refresh. A database failure is 503, not 401: the session is
	// still valid and the client must not treat the outage as a sign-out.
	row, err := s.queries.UserGetByID(r.Context(), db.UserGetByIDParams{ID: claims.UserID()})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.writeErrorJSON(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		s.logger.Error("failed to resolve user on refresh", "error", err)
		s.writeErrorJSON(w, http.StatusServiceUnavailable, "Temporarily unavailable")
		return
	}

	organizationIDs, err := s.getUserOrganizationIDs(r.Context(), row.ID)
	if err != nil {
		s.logger.Error("failed to get user organizations", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to get user organizations")
		return
	}

	u := &user{
		ID:              row.ID,
		OrganizationIDs: organizationIDs,
		Name:            row.Name,
	}

	exp := s.sessionExpiry(sess, now)
	accessToken, err := s.generateJWTWithExpiry(u, claims.Groups, sess, exp)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		s.writeErrorJSON(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	ttl := exp.Sub(now)
	http.SetCookie(w, s.buildAuthCookie(accessToken, ttl))
	if err := s.writeJSON(w, http.StatusOK, authnhttp.RefreshResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(ttl.Seconds()),
	}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}
