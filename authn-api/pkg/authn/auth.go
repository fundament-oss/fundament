package authn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/psqldb"
)

// Ensure AuthnServer implements ServerInterface
var _ authnhttp.ServerInterface = (*AuthnServer)(nil)

// user represents user data for JWT generation.
// This is an internal adapter type between sqlc row types and JWT claims.
type user struct {
	ID              uuid.UUID
	OrganizationIDs []uuid.UUID
	Name            string
	ExternalRef     string
}

// Config holds the configuration for the authentication server.
type Config struct {
	TokenExpiry  time.Duration
	JWTSecret    []byte
	CookieDomain string
	CookieSecure bool
	FrontendURL  string
}

// AuthnServer handles authentication operations.
type AuthnServer struct {
	config        *Config
	oauth2Config  *oauth2.Config
	oidcVerifier  *oidc.IDTokenVerifier
	db            *psqldb.DB
	queries       *db.Queries
	sessionStore  *SessionStore
	logger        *slog.Logger
	validator     *auth.Validator
	cookieBuilder *auth.CookieBuilder
}

// New creates a new AuthnServer.
func New(logger *slog.Logger, cfg *Config, oauth2Config *oauth2.Config, verifier *oidc.IDTokenVerifier, sessionStore *SessionStore, database *psqldb.DB) (*AuthnServer, error) {
	return &AuthnServer{
		config:        cfg,
		logger:        logger,
		oauth2Config:  oauth2Config,
		oidcVerifier:  verifier,
		db:            database,
		queries:       db.New(database.Pool),
		sessionStore:  sessionStore,
		validator:     auth.NewValidator(cfg.JWTSecret, logger),
		cookieBuilder: auth.NewCookieBuilder(cfg.CookieDomain, cfg.CookieSecure),
	}, nil
}

// getUserOrganizationIDs fetches the accepted organization IDs for a user.
func (s *AuthnServer) getUserOrganizationIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	memberships, err := s.queries.UserListOrganizations(ctx, db.UserListOrganizationsParams{UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("listing user organizations: %w", err)
	}

	organizationIDs := make([]uuid.UUID, 0, len(memberships))
	for _, membership := range memberships {
		organizationIDs = append(organizationIDs, membership.OrganizationID)
	}

	return organizationIDs, nil
}

// generateJWT generates a JWT for the given user with the default expiry.
func (s *AuthnServer) generateJWT(u *user, groups []string) (string, error) {
	return s.generateJWTWithExpiry(u, groups, s.config.TokenExpiry)
}

// generateJWTWithExpiry generates a JWT for the given user with a custom expiry.
func (s *AuthnServer) generateJWTWithExpiry(u *user, groups []string, expiry time.Duration) (string, error) {
	now := time.Now()

	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fundament-authn-api",
			Subject:   u.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		UserID:          u.ID,
		OrganizationIDs: u.OrganizationIDs,
		Name:            u.Name,
		Groups:          groups,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.config.JWTSecret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

// StateData holds the OAuth state including CSRF nonce and optional return URL.
type StateData struct {
	Nonce    string `json:"n"`
	ReturnTo string `json:"r,omitempty"`
}

// generateState creates an encoded OAuth state with a CSRF nonce and optional return URL.
func generateState(returnTo string) (string, error) {
	b := make([]byte, 32)

	_, err := rand.Read(b)
	if err != nil {
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

// parseState decodes an OAuth state and returns the StateData.
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

// buildAuthCookie returns the auth cookie with the signed token.
func (s *AuthnServer) buildAuthCookie(token string) *http.Cookie {
	return s.cookieBuilder.Build(token, int(s.config.TokenExpiry.Seconds()))
}

// buildClearAuthCookie returns a cookie that clears the auth cookie.
func (s *AuthnServer) buildClearAuthCookie() *http.Cookie {
	return s.cookieBuilder.BuildClear()
}

// authenticateWithPassword authenticates with OIDC using the password grant flow.
func (s *AuthnServer) authenticateWithPassword(ctx context.Context, email, password string) (*oauth2.Token, error) {
	token, err := s.oauth2Config.PasswordCredentialsToken(ctx, email, password)
	if err != nil {
		return nil, fmt.Errorf("password authentication failed: %w", err)
	}
	return token, nil
}
