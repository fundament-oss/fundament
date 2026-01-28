package auth

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthCookieName is the name of the authentication cookie.
const AuthCookieName = "fundament_auth"

// Claims represents the JWT claims used across fundament services.
type Claims struct {
	jwt.RegisteredClaims
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Groups         []string  `json:"groups"`
	Name           string    `json:"name"`
}

// Validator handles JWT validation from HTTP headers.
type Validator struct {
	jwtSecret []byte
	logger    *slog.Logger
}

// NewValidator creates a new Validator with the given JWT secret.
// Logger is optional and can be nil.
func NewValidator(jwtSecret []byte, logger *slog.Logger) *Validator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Validator{
		jwtSecret: jwtSecret,
		logger:    logger,
	}
}

// Validate validates a JWT from the Authorization header,
// falling back to the auth cookie if no Authorization header is present.
func (v *Validator) Validate(header http.Header) (*Claims, error) {
	// First try Authorization header
	authHeader := header.Get("Authorization")
	if len(authHeader) >= 8 && authHeader[:7] == "Bearer " {
		return v.validateBearer(header)
	}

	// Fall back to cookie from Cookie header
	tokenString := v.extractCookieToken(header)
	if tokenString == "" {
		return nil, fmt.Errorf("no authorization header or auth cookie found")
	}

	return v.validateToken(tokenString)
}

// extractCookieToken extracts the auth token from the Cookie header.
func (v *Validator) extractCookieToken(header http.Header) string {
	cookieHeader := header.Get("Cookie")
	if cookieHeader == "" {
		return ""
	}

	// Cookie header format: "name1=value1; name2=value2"
	for part := range strings.SplitSeq(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, AuthCookieName+"="); ok {
			return after
		}
	}

	return ""
}

// validateToken parses and validates a JWT token string.
func (v *Validator) validateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			v.logger.Debug("unexpected signing method", "alg", token.Header["alg"])
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.jwtSecret, nil
	})
	if err != nil {
		v.logger.Debug("token validation failed", "error", err)
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		v.logger.Debug("invalid token claims")
		return nil, fmt.Errorf("invalid token claims")
	}

	v.logger.Debug("token validated", "user_id", claims.UserID, "organization_id", claims.OrganizationID)
	return claims, nil
}

// validateBearer validates a JWT from the Authorization header.
// Returns an error if the header is missing or the token is invalid.
func (v *Validator) validateBearer(header http.Header) (*Claims, error) {
	authHeader := header.Get("Authorization")
	if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
		v.logger.Debug("missing or invalid authorization header")
		return nil, fmt.Errorf("missing or invalid authorization header")
	}

	tokenString := authHeader[7:]
	return v.validateToken(tokenString)
}
