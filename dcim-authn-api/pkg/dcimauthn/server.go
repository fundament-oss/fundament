package dcimauthn

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/common/auth"
)

// Config holds the configuration for the DCIM auth server.
type Config struct {
	TokenExpiry  time.Duration
	JWTSecret    []byte
	CookieDomain string
	CookieSecure bool
	FrontendURL  string
}

// Server handles DCIM authentication.
type Server struct {
	config        *Config
	oauth2Config  *oauth2.Config
	oidcVerifier  *oidc.IDTokenVerifier
	sessionStore  *SessionStore
	logger        *slog.Logger
	validator     *auth.Validator
	cookieBuilder *auth.CookieBuilder
}

// New creates a new DCIM auth Server.
func New(logger *slog.Logger, cfg *Config, oauth2Config *oauth2.Config, verifier *oidc.IDTokenVerifier, sessionStore *SessionStore) *Server {
	return &Server{
		config:        cfg,
		logger:        logger,
		oauth2Config:  oauth2Config,
		oidcVerifier:  verifier,
		sessionStore:  sessionStore,
		validator:     auth.NewValidator(cfg.JWTSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, logger),
		cookieBuilder: auth.NewCookieBuilder(cfg.CookieDomain, cfg.CookieSecure, auth.DCIMAuthCookieName),
	}
}

// oidcClaims represents the claims extracted from a dex ID token.
type oidcClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Sub   string `json:"sub"`
}

// subjectUUID derives a deterministic UUIDv5 from the OIDC sub claim.
// The Validator requires a UUID subject; dex's sub is not a UUID.
var dcimNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // UUID namespace DNS

func subjectUUID(sub string) uuid.UUID {
	return uuid.NewSHA1(dcimNamespace, []byte(sub))
}

func (s *Server) verifyAndParseIDToken(r *http.Request, rawIDToken string) (*oidcClaims, error) {
	idToken, err := s.oidcVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.logger.Info("ID token verification failed", "error", err)
		return nil, err
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		s.logger.Error("failed to parse claims", "error", err)
		return nil, err
	}

	return &claims, nil
}

func (s *Server) generateJWT(claims *oidcClaims) (string, error) {
	userID := subjectUUID(claims.Sub)
	return s.mintJWT(userID.String(), claims.Name)
}

// generateJWTFromSubject re-issues a token for an already-resolved subject UUID (used in refresh).
func (s *Server) generateJWTFromSubject(claims *oidcClaims) (string, error) {
	return s.mintJWT(claims.Sub, claims.Name)
}

func (s *Server) mintJWT(subject, name string) (string, error) {
	now := time.Now()
	jwtClaims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DCIMIssuer,
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.TokenExpiry)),
		},
		Name: name,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	signed, err := token.SignedString(s.config.JWTSecret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

func (s *Server) buildAuthCookie(token string) *http.Cookie {
	return s.cookieBuilder.Build(token, int(s.config.TokenExpiry.Seconds()))
}

func (s *Server) buildClearAuthCookie() *http.Cookie {
	return s.cookieBuilder.BuildClear()
}

func (s *Server) authenticateWithPassword(r *http.Request, email, password string) (*oauth2.Token, error) {
	token, err := s.oauth2Config.PasswordCredentialsToken(r.Context(), email, password)
	if err != nil {
		return nil, fmt.Errorf("password authentication failed: %w", err)
	}
	return token, nil
}

// StateData holds the OAuth state including CSRF nonce and optional return URL.
type StateData struct {
	Nonce    string `json:"n"`
	ReturnTo string `json:"r,omitempty"`
}

func generateState(returnTo string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random state: %w", err)
	}
	data := StateData{
		Nonce:    base64.URLEncoding.EncodeToString(b),
		ReturnTo: returnTo,
	}
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshaling state: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func parseState(state string) (*StateData, error) {
	jsonBytes, err := base64.URLEncoding.DecodeString(state)
	if err != nil {
		return nil, fmt.Errorf("decoding state: %w", err)
	}
	var data StateData
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("unmarshaling state: %w", err)
	}
	return &data, nil
}

func (s *Server) getRedirectURL(state string) string {
	stateData, err := parseState(state)
	if err != nil {
		s.logger.Warn("failed to parse state for return_to", "error", err)
		return s.config.FrontendURL
	}
	if stateData.ReturnTo != "" && s.isSafeReturnTo(stateData.ReturnTo) {
		return stateData.ReturnTo
	}
	return s.config.FrontendURL
}

// isSafeReturnTo reports whether returnTo is a trusted post-login redirect
// target. Only same-origin absolute URLs (matching the configured frontend) are
// allowed, which prevents using return_to as an open-redirect for phishing.
func (s *Server) isSafeReturnTo(returnTo string) bool {
	target, err := url.Parse(returnTo)
	if err != nil {
		s.logger.Warn("rejecting unparsable return_to", "return_to", returnTo)
		return false
	}

	frontend, err := url.Parse(s.config.FrontendURL)
	if err != nil {
		s.logger.Error("invalid configured frontend URL", "frontend_url", s.config.FrontendURL, "error", err)
		return false
	}

	if target.Scheme != frontend.Scheme || target.Host != frontend.Host {
		s.logger.Warn("rejecting cross-origin return_to", "return_to", returnTo)
		return false
	}

	return true
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}
