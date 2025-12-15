package main

import (
	"net/http"

	"github.com/gorilla/sessions"
)

const (
	sessionName           = "fundament_session"
	sessionKeyState       = "oauth_state"
	sessionKeyPKCEVerifier = "pkce_verifier"
)

// SessionStore wraps gorilla/sessions for OAuth state management.
// TODO: Replace CookieStore with Redis store for production multi-instance deployments.
type SessionStore struct {
	store *sessions.CookieStore
}

// NewSessionStore creates a new session store with the given secret key.
func NewSessionStore(secret []byte) *SessionStore {
	store := sessions.NewCookieStore(secret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   600, // 10 minutes for OAuth state
		HttpOnly: true,
		Secure:   false, // Set to true in production
		SameSite: http.SameSiteLaxMode,
	}
	return &SessionStore{store: store}
}

// SetOAuthParams stores the OAuth state and PKCE verifier in the session.
func (s *SessionStore) SetOAuthParams(w http.ResponseWriter, r *http.Request, state, verifier string) error {
	session, err := s.store.Get(r, sessionName)
	if err != nil {
		// Ignore decode errors for new sessions
		session, _ = s.store.New(r, sessionName)
	}
	session.Values[sessionKeyState] = state
	session.Values[sessionKeyPKCEVerifier] = verifier
	return session.Save(r, w)
}

// VerifyStateAndGetVerifier checks if the provided state matches the stored state,
// returns the PKCE verifier, and clears both from the session.
func (s *SessionStore) VerifyStateAndGetVerifier(w http.ResponseWriter, r *http.Request, state string) (string, bool, error) {
	session, err := s.store.Get(r, sessionName)
	if err != nil {
		return "", false, nil
	}

	storedState, ok := session.Values[sessionKeyState].(string)
	if !ok || storedState != state {
		return "", false, nil
	}

	verifier, _ := session.Values[sessionKeyPKCEVerifier].(string)

	// Clear state and verifier after verification (single use)
	delete(session.Values, sessionKeyState)
	delete(session.Values, sessionKeyPKCEVerifier)
	if err := session.Save(r, w); err != nil {
		return "", false, err
	}

	return verifier, true, nil
}

// ConfigureOptions allows customizing session options.
func (s *SessionStore) ConfigureOptions(domain string, secure bool) {
	// Don't set domain for localhost - browsers handle it better without explicit domain
	if domain != "" && domain != "localhost" {
		s.store.Options.Domain = domain
	}
	s.store.Options.Secure = secure
}
