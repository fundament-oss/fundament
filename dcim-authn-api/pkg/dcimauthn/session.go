package dcimauthn

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/gorilla/sessions"
)

const (
	sessionName            = "dcim_session"
	sessionKeyState        = "oauth_state"
	sessionKeyPKCEVerifier = "pkce_verifier"
)

// SessionStore wraps gorilla/sessions for OAuth state management.
type SessionStore struct {
	store *sessions.CookieStore
}

// deriveKey derives a fixed-length key from secret using HKDF-SHA256 with a
// distinct info label, so that keys used for different purposes are
// cryptographically independent even when seeded from the same input secret.
func deriveKey(secret []byte, info string, length int) ([]byte, error) {
	return hkdf.Key(sha256.New, secret, nil, info, length)
}

// NewSessionStore creates a new session store keyed from the given secret.
// The cookie store's authentication (hash) and encryption (block) keys are
// derived from secret via HKDF rather than using it directly, which keeps them
// independent from the JWT signing secret and encrypts the stored OAuth state.
func NewSessionStore(secret []byte) (*SessionStore, error) {
	hashKey, err := deriveKey(secret, "dcim-authn-session-hash", 32)
	if err != nil {
		return nil, fmt.Errorf("deriving session hash key: %w", err)
	}
	blockKey, err := deriveKey(secret, "dcim-authn-session-block", 32)
	if err != nil {
		return nil, fmt.Errorf("deriving session block key: %w", err)
	}

	store := sessions.NewCookieStore(hashKey, blockKey)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   600, // 10 minutes for OAuth state
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	}
	return &SessionStore{store: store}, nil
}

// ConfigureOptions allows customizing session options.
func (s *SessionStore) ConfigureOptions(domain string, secure bool) {
	if domain != "" && domain != "localhost" {
		s.store.Options.Domain = domain
	}
	s.store.Options.Secure = secure
}

// SetOAuthParams stores the OAuth state and PKCE verifier in the session.
func (s *SessionStore) SetOAuthParams(w http.ResponseWriter, r *http.Request, state, verifier string) error {
	session, err := s.store.Get(r, sessionName)
	if err != nil {
		return fmt.Errorf("could not retrieve session: %w", err)
	}

	session.Values[sessionKeyState] = state
	session.Values[sessionKeyPKCEVerifier] = verifier

	if err := session.Save(r, w); err != nil {
		return fmt.Errorf("saving session: %w", err)
	}

	return nil
}

// VerifyStateAndGetVerifier checks if the provided state matches the stored state,
// returns the PKCE verifier, and clears both from the session.
func (s *SessionStore) VerifyStateAndGetVerifier(w http.ResponseWriter, r *http.Request, state string) (string, bool, error) {
	session, err := s.store.Get(r, sessionName)
	if err != nil {
		return "", false, fmt.Errorf("get session: %w", err)
	}

	storedState, ok := session.Values[sessionKeyState].(string)
	if !ok || storedState == "" {
		return "", false, fmt.Errorf("state not found in session")
	}

	if storedState != state {
		return "", false, nil
	}

	verifier, ok := session.Values[sessionKeyPKCEVerifier].(string)
	if !ok || verifier == "" {
		return "", false, fmt.Errorf("PKCE verifier not found in session")
	}

	delete(session.Values, sessionKeyState)
	delete(session.Values, sessionKeyPKCEVerifier)
	if err := session.Save(r, w); err != nil {
		return "", false, fmt.Errorf("saving session: %w", err)
	}

	return verifier, true, nil
}
