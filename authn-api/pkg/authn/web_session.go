package authn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/auth"
)

const (
	// refreshTokenBytes is the size of a refresh token's random part. It is not
	// a JWT and carries no structure: only authn-api ever reads it, and a token
	// that cannot be parsed cannot be mistaken for an access token by a service
	// that happens to see it (FUN-23).
	refreshTokenBytes = 32

	// refreshTokenPrefixLength is how much of a token is stored in the clear, as
	// token_prefix, so that a log line can name a session without naming its
	// credential. Mirrors authn.api_keys.
	refreshTokenPrefixLength = 8
)

// webSessionStore is the subset of db.Queries the browser-session code uses,
// extracted for the same reason as authzEvaluator and clusterLookup: the
// rotation and revocation rules are worth testing without standing up a
// Postgres, while the SQL behind them is exercised end to end.
type webSessionStore interface {
	WebSessionCreate(ctx context.Context, arg db.WebSessionCreateParams) (db.WebSessionCreateRow, error)
	WebSessionGetByHash(ctx context.Context, arg db.WebSessionGetByHashParams) (db.WebSessionGetByHashRow, error)
	WebSessionRotate(ctx context.Context, arg db.WebSessionRotateParams) (db.WebSessionRotateRow, error)
	WebSessionRevoke(ctx context.Context, arg db.WebSessionRevokeParams) (int64, error)
}

// errSessionTokenSpent reports a refresh token that was already exchanged: a
// replay, or the loser of two refreshes racing on the same token. The caller
// cannot tell which party is the user, so it revokes the chain.
var errSessionTokenSpent = errors.New("refresh token already spent")

// newRefreshToken mints a refresh token and returns it with the hash to store
// and the prefix to log. The token itself is never stored.
func newRefreshToken() (token string, hash []byte, prefix string, err error) {
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, "", fmt.Errorf("generating refresh token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashRefreshToken(token), token[:refreshTokenPrefixLength], nil
}

// hashRefreshToken hashes a refresh token for storage and lookup. SHA-256
// without a salt on purpose: the token is 32 random bytes, so there is nothing
// to guess and a per-row salt would only make the lookup by hash impossible.
func hashRefreshToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// webSession is what a handler needs from a session row to mint a token from
// it, flattened out of the sqlc row types that the create and rotate queries
// return separately.
type webSession struct {
	// ID is the row — one refresh token's worth of the session.
	ID uuid.UUID
	// SessionID names the whole chain, and is what a logout revokes and what
	// rides in a token's `sid` claim.
	SessionID uuid.UUID
	UserID    uuid.UUID
	Groups    []string
	// RefreshToken is the token the browser must be given. Present only on a
	// session just created or just rotated; it is never read back from the
	// database.
	RefreshToken string
}

// createWebSession starts a session for a signed-in user, returning it with the
// refresh token to set as a cookie.
func (s *AuthnServer) createWebSession(ctx context.Context, userID uuid.UUID, groups []string) (*webSession, error) {
	token, hash, prefix, err := newRefreshToken()
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generating session id: %w", err)
	}

	row, err := s.webSessions.WebSessionCreate(ctx, db.WebSessionCreateParams{
		ID:          id,
		UserID:      userID,
		TokenHash:   hash,
		TokenPrefix: prefix,
		Groups:      groups,
		Expires:     s.sessionExpiry(),
	})
	if err != nil {
		return nil, fmt.Errorf("creating web session: %w", err)
	}

	s.logger.Info("web session started",
		"user_id", userID,
		"session_id", row.SessionID,
		"token_prefix", prefix,
	)

	return &webSession{
		ID:           row.ID,
		SessionID:    row.SessionID,
		UserID:       row.UserID,
		Groups:       row.Groups,
		RefreshToken: token,
	}, nil
}

// signIn starts a browser session for a user the identity provider has just
// vouched for, and mints the first access token from it. The caller sets both
// cookies: the access cookie the whole platform reads, and the refresh cookie
// only this service's refresh endpoint ever sees.
func (s *AuthnServer) signIn(ctx context.Context, u *user, groups []string) (string, *webSession, error) {
	session, err := s.createWebSession(ctx, u.ID, groups)
	if err != nil {
		return "", nil, err
	}

	accessToken, err := s.generateJWT(u, groups, session.SessionID.String())
	if err != nil {
		return "", nil, fmt.Errorf("generating JWT: %w", err)
	}

	return accessToken, session, nil
}

// spendRefreshToken exchanges a refresh token for its successor.
//
// Every failure here is the end of the session rather than a retryable error: a
// token that is unknown, expired, revoked or already spent cannot be made good
// by presenting it again, and the two cases that mean a copy is in circulation
// — a spent token, and a token whose row the rotation lost the race for — take
// the whole chain with them.
func (s *AuthnServer) spendRefreshToken(ctx context.Context, token string) (*webSession, error) {
	row, err := s.webSessions.WebSessionGetByHash(ctx, db.WebSessionGetByHashParams{
		TokenHash: hashRefreshToken(token),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("unknown refresh token")
		}
		return nil, fmt.Errorf("looking up web session: %w", err)
	}

	if row.Revoked.Valid {
		return nil, fmt.Errorf("session revoked at %s", row.Revoked.Time)
	}

	// Spent: two parties hold this token and neither is identifiable as the
	// user, so the chain goes rather than the token.
	if row.RotatedTo.Valid {
		s.logger.Warn("refresh token reused after rotation",
			"user_id", row.UserID,
			"session_id", row.SessionID,
			"token_prefix", row.TokenPrefix,
		)
		s.revokeWebSession(ctx, row.SessionID, "refresh token reuse")
		return nil, errSessionTokenSpent
	}

	now := time.Now()
	if row.Expires.Valid && now.After(row.Expires.Time) {
		return nil, fmt.Errorf("session idle since %s", row.Expires.Time)
	}
	if row.Started.Valid && now.Sub(row.Started.Time) > s.config.SessionMaxLifetime {
		return nil, fmt.Errorf("session older than %s", s.config.SessionMaxLifetime)
	}

	successor, hash, prefix, err := newRefreshToken()
	if err != nil {
		return nil, err
	}

	newID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generating session id: %w", err)
	}

	rotated, err := s.webSessions.WebSessionRotate(ctx, db.WebSessionRotateParams{
		ID:          row.ID,
		NewID:       newID,
		TokenHash:   hash,
		TokenPrefix: prefix,
		Expires:     s.sessionExpiry(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The row was fine a moment ago, so something else spent, revoked
			// or ended it in between. Treated as reuse: the same two-holders
			// reasoning applies, and it is the safe reading of the race.
			s.logger.Warn("refresh token spent concurrently",
				"user_id", row.UserID,
				"session_id", row.SessionID,
				"token_prefix", row.TokenPrefix,
			)
			s.revokeWebSession(ctx, row.SessionID, "concurrent refresh")
			return nil, errSessionTokenSpent
		}
		return nil, fmt.Errorf("rotating web session: %w", err)
	}

	return &webSession{
		ID:           rotated.ID,
		SessionID:    rotated.SessionID,
		UserID:       rotated.UserID,
		Groups:       rotated.Groups,
		RefreshToken: successor,
	}, nil
}

// revokeWebSession ends a session and every token in its chain. It reports
// rather than returns failure: every caller is on its way to clearing the
// browser's cookies regardless, and a logout that answered an error because the
// database was briefly unreachable would leave the user looking signed in.
func (s *AuthnServer) revokeWebSession(ctx context.Context, sessionID uuid.UUID, reason string) {
	revoked, err := s.webSessions.WebSessionRevoke(ctx, db.WebSessionRevokeParams{
		SessionID: sessionID,
	})
	if err != nil {
		s.logger.Error("failed to revoke web session",
			"error", err, "session_id", sessionID, "reason", reason)
		return
	}

	s.logger.Info("web session revoked",
		"session_id", sessionID, "reason", reason, "tokens_revoked", revoked)
}

// sessionExpiry is when a refresh token minted now goes idle. It moves forward
// on every rotation, so the idle timeout measures time since the last refresh,
// while SessionMaxLifetime measures time since sign-in and does not move.
func (s *AuthnServer) sessionExpiry() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(s.config.SessionIdleTimeout), Valid: true}
}

// refreshTokenFromRequest reads the refresh cookie, or "" when the browser sent
// none. There is no header fallback on purpose: a refresh token belongs in the
// cookie the browser cannot read and cannot send anywhere else, and accepting
// one in a header would invite clients to store it where a script can reach it.
func refreshTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(auth.ConsoleRefreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// buildRefreshCookie returns the refresh cookie for a token. Its lifetime is
// the session's idle timeout, so a browser that comes back to an idle-expired
// session has already dropped the cookie instead of presenting a token that
// cannot work.
func (s *AuthnServer) buildRefreshCookie(token string) *http.Cookie {
	return s.refreshCookieBuilder.Build(token, int(s.config.SessionIdleTimeout.Seconds()))
}
