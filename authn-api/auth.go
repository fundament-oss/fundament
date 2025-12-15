package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/sqlc/db"
)

const AuthCookieName = "fundament_auth"

type Claims struct {
	jwt.RegisteredClaims
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Groups   []string  `json:"groups"`
	Name     string    `json:"name"`
}

func (s *AuthnServer) generateJWT(user *db.OrganizationUser, groups []string) (string, error) {
	now := time.Now()

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fundament-authn-api",
			Subject:   user.ExternalID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.TokenExpiry)),
		},
		UserID:   user.ID,
		TenantID: user.TenantID,
		Name:     user.Name,
		Groups:   groups,
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

	s.logger.Debug("token validated", "user_id", claims.UserID, "tenant_id", claims.TenantID)
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
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random state: %w", err)
	}

	data := StateData{
		Nonce:    base64.URLEncoding.EncodeToString(b),
		ReturnTo: returnTo,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshaling state: %w", err)
	}

	fmt.Printf("jsonBytes: %v\n", string(jsonBytes))
	fmt.Printf("jsonBytes: %v\n", string(base64.URLEncoding.EncodeToString(jsonBytes)))

	return base64.URLEncoding.EncodeToString(jsonBytes), nil
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
	// Find the last dot to separate value from signature
	lastDot := strings.LastIndex(signedValue, ".")
	if lastDot == -1 {
		return "", fmt.Errorf("invalid signed value format: no signature separator")
	}

	value := signedValue[:lastDot]
	signature := signedValue[lastDot+1:]

	mac := hmac.New(sha256.New, s.config.JWTSecret)
	mac.Write([]byte(value))
	expectedSig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid signature")
	}

	return value, nil
}

// getCookieDomain returns the domain for cookies, empty for localhost
func (s *AuthnServer) getCookieDomain() string {
	// Don't set domain for localhost - browsers handle it better without explicit domain
	if s.config.CookieDomain == "localhost" {
		return ""
	}
	return s.config.CookieDomain
}

// setAuthCookie sets the signed auth cookie with the JWT token
func (s *AuthnServer) setAuthCookie(w http.ResponseWriter, token string) {
	signedToken := s.signCookieValue(token)
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    signedToken,
		Path:     "/",
		Domain:   s.getCookieDomain(),
		MaxAge:   int(s.config.TokenExpiry.Seconds()),
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearAuthCookie removes the auth cookie
func (s *AuthnServer) clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.getCookieDomain(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
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

// authenticateWithPassword authenticates with Dex using the password grant flow.
// This uses Dex's Resource Owner Password Credentials (ROPC) flow via the token endpoint.
func (s *AuthnServer) authenticateWithPassword(ctx context.Context, email, password string) (*oauth2.Token, error) {
	// Use the password grant type with Dex
	// Dex expects: grant_type=password&username=<email>&password=<password>&scope=openid profile email groups
	token, err := s.oauth2Config.PasswordCredentialsToken(ctx, email, password)
	if err != nil {
		return nil, fmt.Errorf("password authentication failed: %w", err)
	}
	return token, nil
}
