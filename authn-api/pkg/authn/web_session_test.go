package authn

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/auth"
)

// fakeWebSessions stands in for the four session queries. It records what was
// asked of it rather than imitating Postgres: the rotation and revocation rules
// under test are the handler's, and the SQL that enforces them under
// concurrency is exercised end to end instead.
type fakeWebSessions struct {
	row        db.WebSessionGetByHashRow
	getErr     error
	rotateErr  error
	revokedIDs []uuid.UUID
}

//nolint:gocritic // the parameter is by value because db.Queries' method is
func (f *fakeWebSessions) WebSessionCreate(_ context.Context, arg db.WebSessionCreateParams) (db.WebSessionCreateRow, error) {
	return db.WebSessionCreateRow{
		ID:          arg.ID,
		SessionID:   arg.ID,
		UserID:      arg.UserID,
		TokenPrefix: arg.TokenPrefix,
		Groups:      arg.Groups,
		Expires:     arg.Expires,
	}, nil
}

func (f *fakeWebSessions) WebSessionGetByHash(_ context.Context, _ db.WebSessionGetByHashParams) (db.WebSessionGetByHashRow, error) {
	if f.getErr != nil {
		return db.WebSessionGetByHashRow{}, f.getErr
	}
	return f.row, nil
}

//nolint:gocritic // the parameter is by value because db.Queries' method is
func (f *fakeWebSessions) WebSessionRotate(_ context.Context, arg db.WebSessionRotateParams) (db.WebSessionRotateRow, error) {
	if f.rotateErr != nil {
		return db.WebSessionRotateRow{}, f.rotateErr
	}
	return db.WebSessionRotateRow{
		ID:          arg.NewID,
		SessionID:   f.row.SessionID,
		UserID:      f.row.UserID,
		TokenPrefix: arg.TokenPrefix,
		Groups:      f.row.Groups,
		Started:     f.row.Started,
		Expires:     arg.Expires,
	}, nil
}

func (f *fakeWebSessions) WebSessionRevoke(_ context.Context, arg db.WebSessionRevokeParams) (int64, error) {
	f.revokedIDs = append(f.revokedIDs, arg.SessionID)
	return 1, nil
}

func testSessionServer(t *testing.T, sessions webSessionStore) *AuthnServer {
	t.Helper()
	return &AuthnServer{
		config: &Config{
			JWTSecret:          []byte("test-secret"),
			TokenExpiry:        10 * time.Minute,
			SessionIdleTimeout: 24 * time.Hour,
			SessionMaxLifetime: 7 * 24 * time.Hour,
		},
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		webSessions: sessions,
		cookieBuilder: auth.NewCookieBuilder(
			"fundament.localhost", true, auth.ConsoleAuthCookieName),
		refreshCookieBuilder: auth.NewHostOnlyCookieBuilder(
			true, auth.ConsoleRefreshCookieName, auth.ConsoleRefreshCookiePath),
	}
}

// liveSession is a row that spendRefreshToken should accept.
func liveSession() db.WebSessionGetByHashRow {
	id := uuid.New()
	return db.WebSessionGetByHashRow{
		ID:          id,
		SessionID:   id,
		UserID:      uuid.New(),
		TokenPrefix: "abcdefgh",
		Groups:      []string{"platform-admins"},
		Started:     pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Expires:     pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}
}

func TestNewRefreshToken_IsOpaqueAndHashed(t *testing.T) {
	token, hash, prefix, err := newRefreshToken()
	require.NoError(t, err)

	second, _, _, err := newRefreshToken()
	require.NoError(t, err)

	assert.NotEqual(t, token, second, "every session gets its own token")
	assert.Len(t, hash, 32, "SHA-256 of the token is what is stored")
	assert.Equal(t, token[:refreshTokenPrefixLength], prefix)
	assert.Equal(t, hash, hashRefreshToken(token), "the stored hash is what a lookup recomputes")
	assert.NotContains(t, token, ".", "not a JWT: nothing may try to parse it as one")
}

func TestSpendRefreshToken_RotatesALiveSession(t *testing.T) {
	sessions := &fakeWebSessions{row: liveSession()}
	server := testSessionServer(t, sessions)

	rotated, err := server.spendRefreshToken(t.Context(), "a-token")
	require.NoError(t, err)

	assert.Equal(t, sessions.row.SessionID, rotated.SessionID, "the chain survives a rotation")
	assert.NotEqual(t, sessions.row.ID, rotated.ID, "the token does not")
	assert.NotEmpty(t, rotated.RefreshToken, "the browser is handed the successor")
	assert.Equal(t, sessions.row.Groups, rotated.Groups,
		"groups ride on the row: a refresh has no ID token to re-read them from")
	assert.Empty(t, sessions.revokedIDs, "a legitimate refresh revokes nothing")
}

func TestSpendRefreshToken_ReuseRevokesTheChain(t *testing.T) {
	spent := liveSession()
	spent.RotatedTo = pgtype.UUID{Bytes: uuid.New(), Valid: true}

	sessions := &fakeWebSessions{row: spent}
	server := testSessionServer(t, sessions)

	_, err := server.spendRefreshToken(t.Context(), "a-token")
	require.ErrorIs(t, err, errSessionTokenSpent)
	assert.Equal(t, []uuid.UUID{spent.SessionID}, sessions.revokedIDs,
		"two parties hold a spent token, so the session goes rather than the token")
}

// A rotation that matches no row means the token was spent between the lookup
// and the update, which is the same two-holders situation as an outright replay.
func TestSpendRefreshToken_LostRaceRevokesTheChain(t *testing.T) {
	sessions := &fakeWebSessions{row: liveSession(), rotateErr: pgx.ErrNoRows}
	server := testSessionServer(t, sessions)

	_, err := server.spendRefreshToken(t.Context(), "a-token")
	require.ErrorIs(t, err, errSessionTokenSpent)
	assert.Len(t, sessions.revokedIDs, 1)
}

func TestSpendRefreshToken_RefusesDeadSessions(t *testing.T) {
	tests := map[string]func(db.WebSessionGetByHashRow) db.WebSessionGetByHashRow{
		"revoked": func(row db.WebSessionGetByHashRow) db.WebSessionGetByHashRow {
			row.Revoked = pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true}
			return row
		},
		"idle past expires": func(row db.WebSessionGetByHashRow) db.WebSessionGetByHashRow {
			row.Expires = pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true}
			return row
		},
		"older than the maximum lifetime": func(row db.WebSessionGetByHashRow) db.WebSessionGetByHashRow {
			row.Started = pgtype.Timestamptz{Time: time.Now().Add(-8 * 24 * time.Hour), Valid: true}
			return row
		},
	}

	for name, mangle := range tests {
		t.Run(name, func(t *testing.T) {
			sessions := &fakeWebSessions{row: mangle(liveSession())}
			server := testSessionServer(t, sessions)

			_, err := server.spendRefreshToken(t.Context(), "a-token")
			require.Error(t, err)
			assert.NotErrorIs(t, err, errSessionTokenSpent,
				"a session that simply ran out is not evidence of a second holder")
		})
	}
}

func TestSpendRefreshToken_UnknownTokenIsRefused(t *testing.T) {
	sessions := &fakeWebSessions{getErr: pgx.ErrNoRows}
	server := testSessionServer(t, sessions)

	_, err := server.spendRefreshToken(t.Context(), "a-token")
	require.Error(t, err)
	assert.Empty(t, sessions.revokedIDs, "there is no chain to revoke")
}

func TestRefreshCookie_IsHostOnlyAndScopedToRefresh(t *testing.T) {
	server := testSessionServer(t, &fakeWebSessions{})

	refresh := server.buildRefreshCookie("a-token")
	assert.Equal(t, auth.ConsoleRefreshCookieName, refresh.Name)
	assert.Empty(t, refresh.Domain, "host-only: no sibling subdomain may receive it")
	assert.Equal(t, auth.ConsoleRefreshCookiePath, refresh.Path,
		"not even this service's other endpoints receive it")
	assert.True(t, refresh.HttpOnly)
	assert.True(t, refresh.Secure)
	assert.Equal(t, http.SameSiteStrictMode, refresh.SameSite)
	assert.Equal(t, int(24*time.Hour/time.Second), refresh.MaxAge,
		"the cookie is dropped when the session would have gone idle anyway")

	// The access cookie is the deliberate opposite: every surface validates it,
	// so every surface receives it, which is why it is the short-lived one.
	access := server.buildAuthCookie("a-jwt")
	assert.Equal(t, "fundament.localhost", access.Domain)
	assert.Equal(t, "/", access.Path)
}

func TestLogout_RevokesTheSessionItsCookieNames(t *testing.T) {
	sessions := &fakeWebSessions{row: liveSession()}
	server := testSessionServer(t, sessions)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/logout", http.NoBody)
	req.AddCookie(&http.Cookie{
		Name: auth.ConsoleRefreshCookieName, Value: "a-token",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	rec := httptest.NewRecorder()

	server.HandleLogout(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []uuid.UUID{sessions.row.SessionID}, sessions.revokedIDs)
	assertCookiesCleared(t, rec)
}

// A logout without the cookie is still a logout: there is nothing to prove and
// nothing to revoke, and answering differently would report which cookies the
// caller holds.
func TestLogout_WithoutRefreshCookieStillClearsAndSucceeds(t *testing.T) {
	sessions := &fakeWebSessions{row: liveSession()}
	server := testSessionServer(t, sessions)

	rec := httptest.NewRecorder()
	server.HandleLogout(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/logout", http.NoBody))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, sessions.revokedIDs, "no cookie names no session")
	assertCookiesCleared(t, rec)
}

func TestLogout_UnknownRefreshTokenStillSucceeds(t *testing.T) {
	sessions := &fakeWebSessions{getErr: pgx.ErrNoRows}
	server := testSessionServer(t, sessions)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/logout", http.NoBody)
	req.AddCookie(&http.Cookie{
		Name: auth.ConsoleRefreshCookieName, Value: "stale",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	rec := httptest.NewRecorder()

	server.HandleLogout(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assertCookiesCleared(t, rec)
}

// A refresh whose session is gone is answered 401 with both cookies cleared, so
// the browser stops presenting a credential that cannot work.
func TestRefresh_RefusedSessionClearsBothCookies(t *testing.T) {
	spent := liveSession()
	spent.RotatedTo = pgtype.UUID{Bytes: uuid.New(), Valid: true}

	server := testSessionServer(t, &fakeWebSessions{row: spent})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/refresh", http.NoBody)
	req.AddCookie(&http.Cookie{
		Name: auth.ConsoleRefreshCookieName, Value: "a-token",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	rec := httptest.NewRecorder()

	server.HandleRefresh(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertCookiesCleared(t, rec)
}

func assertCookiesCleared(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	cleared := map[string]bool{}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
	}

	assert.True(t, cleared[auth.ConsoleAuthCookieName], "the access cookie is cleared")
	assert.True(t, cleared[auth.ConsoleRefreshCookieName], "the refresh cookie is cleared")
}
