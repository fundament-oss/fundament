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

// Claims represents the JWT claims used across Fundament services.
type Claims struct {
	jwt.RegisteredClaims
	UserID          uuid.UUID   `json:"user_id"`
	OrganizationIDs []uuid.UUID `json:"organization_ids"`
	Groups          []string    `json:"groups"`
	Name            string      `json:"name"`
}

// Validator handles JWT validation.
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

// Validate validates a JWT from HTTP headers, trying the Authorization header
// first and falling back to the auth cookie.
func (v *Validator) Validate(header http.Header) (*Claims, error) {
	tokenString := extractTokenFromHeaders(header)
	if tokenString == "" {
		return nil, fmt.Errorf("no authorization header or auth cookie found")
	}
	return v.ValidateToken(tokenString)
}

// ValidateToken parses and validates a JWT token string.
func (v *Validator) ValidateToken(tokenString string) (*Claims, error) {
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

	v.logger.Debug("token validated", "user_id", claims.UserID, "organization_ids", claims.OrganizationIDs)
	return claims, nil
}

func extractTokenFromHeaders(header http.Header) string {
	if token, ok := strings.CutPrefix(header.Get("Authorization"), "Bearer "); ok {
		return token
	}

	cookieHeader := header.Get("Cookie")
	if cookieHeader == "" {
		return ""
	}

	for part := range strings.SplitSeq(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, AuthCookieName+"="); ok {
			return after
		}
	}

	return ""
}
