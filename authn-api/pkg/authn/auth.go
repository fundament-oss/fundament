package authn

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type Config struct {
	TokenExpiry  time.Duration
	JWTSecret    []byte
	CookieDomain string
	CookieSecure bool
	FrontendURL  string
}

type AuthnServer struct {
	config       *Config
	oauth2Config *oauth2.Config
	oidcVerifier *oidc.IDTokenVerifier
	queries      *db.Queries
	sessionStore *SessionStore
	logger       *slog.Logger
}

func New(logger *slog.Logger, cfg *Config, oauth2Config *oauth2.Config, verifier *oidc.IDTokenVerifier, sessionStore *SessionStore, database *psqldb.DB) (*AuthnServer, error) {
	return &AuthnServer{
		config:       cfg,
		logger:       logger,
		oauth2Config: oauth2Config,
		oidcVerifier: verifier,
		queries:      db.New(database.Pool),
		sessionStore: sessionStore,
	}, nil
}

const AuthCookieName = "fundament_auth"

type Claims struct {
	jwt.RegisteredClaims
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Groups         []string  `json:"groups"`
	Name           string    `json:"name"`
}

func (s *AuthnServer) generateJWT(user *db.TenantUser, groups []string) (string, error) {
	now := time.Now()

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fundament-authn-api",
			Subject:   user.ExternalID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.TokenExpiry)),
		},
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Name:           user.Name,
		Groups:         groups,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.config.JWTSecret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

func (s *AuthnServer) validateRequest(header http.Header) (*Claims, error) {
	authHeader := header.Get("Authorization")
	if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
		s.logger.Debug("missing or invalid authorization header")
		return nil, fmt.Errorf("missing or invalid authorization header")
	}

	tokenString := authHeader[7:]

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			s.logger.Debug("unexpected signing method", "alg", token.Header["alg"])
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.config.JWTSecret, nil
	})

	if err != nil {
		s.logger.Debug("token validation failed", "error", err)
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		s.logger.Debug("invalid token claims")
		return nil, fmt.Errorf("invalid token claims")
	}

	s.logger.Debug("token validated", "user_id", claims.UserID, "organization_id", claims.OrganizationID)
	return claims, nil
}

// StateData holds the OAuth state including CSRF nonce and optional return URL.
type StateData struct {
	Nonce    string `json:"n"`
	ReturnTo string `json:"r,omitempty"`
}

// generateState creates an encoded OAuth state with a CSRF nonce and optional return URL.
// If returnTo is empty, the default frontend URL will be used after login.
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

// getDevGroups returns groups for dev/test users when OIDC provider doesn't support groups.
// In production, groups should come from the OIDC provider (e.g., LDAP, Azure AD).
func getDevGroups(email string) []string {
	devGroups := map[string][]string{
		"admin@example.com": {"admins", "users"},
	}
	if groups, ok := devGroups[email]; ok {
		return groups
	}
	return []string{"users"}
}

// signCookieValue signs a value using HMAC-SHA256 and returns "value.signature"
func (s *AuthnServer) signCookieValue(value string) string {
	mac := hmac.New(sha256.New, s.config.JWTSecret)
	mac.Write([]byte(value))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	return value + "." + signature
}

// verifyCookieValue verifies a signed cookie value and returns the original value
// Format is: <value>.<signature> where value may contain dots (e.g., JWT)
func (s *AuthnServer) verifyCookieValue(signedValue string) (string, error) {
	return auth.VerifyCookieValue(signedValue, s.config.JWTSecret)
}

// getCookieDomain returns the domain for cookies, empty for localhost
func (s *AuthnServer) getCookieDomain() string {
	// Don't set domain for localhost - browsers handle it better without explicit domain
	if s.config.CookieDomain == "localhost" {
		return ""
	}
	return s.config.CookieDomain
}

// buildAuthCookie returns the auth cookie with the signed token.
func (s *AuthnServer) buildAuthCookie(token string) *http.Cookie {
	signedToken := s.signCookieValue(token)
	return &http.Cookie{
		Name:     AuthCookieName,
		Value:    signedToken,
		Path:     "/",
		Domain:   s.getCookieDomain(),
		MaxAge:   int(s.config.TokenExpiry.Seconds()),
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

// buildClearAuthCookie returns a cookie that clears the auth cookie.
func (s *AuthnServer) buildClearAuthCookie() *http.Cookie {
	return &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.getCookieDomain(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

// validateRequestOrCookie validates auth from either Authorization header or Cookie header
// This works with http.Header which is used by Connect RPC
func (s *AuthnServer) validateRequestOrCookie(header http.Header) (*Claims, error) {
	// First try Authorization header
	authHeader := header.Get("Authorization")
	if len(authHeader) >= 8 && authHeader[:7] == "Bearer " {
		return s.validateRequest(header)
	}

	// Fall back to cookie from Cookie header
	cookieHeader := header.Get("Cookie")
	if cookieHeader == "" {
		return nil, fmt.Errorf("no authorization header or cookie found")
	}

	// Parse cookies from header
	// Cookie header format: "name1=value1; name2=value2"
	tokenValue := ""
	for _, part := range strings.Split(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, AuthCookieName+"=") {
			tokenValue = strings.TrimPrefix(part, AuthCookieName+"=")
			break
		}
	}

	if tokenValue == "" {
		return nil, fmt.Errorf("auth cookie not found")
	}

	// Verify cookie signature
	tokenString, err := s.verifyCookieValue(tokenValue)
	if err != nil {
		return nil, fmt.Errorf("invalid cookie signature: %w", err)
	}

	// Parse and validate JWT
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.config.JWTSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// authenticateWithPassword authenticates with OIDC using the password grant flow.
func (s *AuthnServer) authenticateWithPassword(ctx context.Context, email, password string) (*oauth2.Token, error) {
	// Use the password grant type
	token, err := s.oauth2Config.PasswordCredentialsToken(ctx, email, password)
	if err != nil {
		return nil, fmt.Errorf("password authentication failed: %w", err)
	}

	return token, nil
}
