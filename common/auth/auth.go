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

// TokenType is the value carried in the JWT `aud` claim. It distinguishes
// user-session tokens from plugin-delegation tokens so that services can
// refuse the wrong kind at validation time.
type TokenType string

const (
	TokenTypeUser   TokenType = "fundament-user"
	TokenTypePlugin TokenType = "fundament-plugin"
)

// Claims represents the JWT claims used across fundament services.
type Claims struct {
	jwt.RegisteredClaims
	OrganizationIDs []uuid.UUID `json:"organization_ids"`
	Groups          []string    `json:"groups"`
	Name            string      `json:"name"`
}

func (c *Claims) UserID() uuid.UUID {
	return uuid.MustParse(c.Subject)
}

// Type returns the token type from the first audience claim, or empty if none.
func (c *Claims) Type() TokenType {
	if len(c.Audience) == 0 {
		return ""
	}
	return TokenType(c.Audience[0])
}

// Validator handles JWT validation from HTTP headers.
type Validator struct {
	jwtSecret        []byte
	expectedAudience TokenType // empty = accept any audience (legacy)
	logger           *slog.Logger
}

// NewValidator creates a Validator that accepts any audience. Prefer
// NewValidatorForAudience in new code so that services explicitly declare
// the token type they accept.
func NewValidator(jwtSecret []byte, logger *slog.Logger) *Validator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Validator{
		jwtSecret: jwtSecret,
		logger:    logger,
	}
}

// NewValidatorForAudience creates a Validator that requires the JWT `aud`
// claim to contain the given TokenType.
func NewValidatorForAudience(jwtSecret []byte, audience TokenType, logger *slog.Logger) *Validator {
	v := NewValidator(jwtSecret, logger)
	v.expectedAudience = audience
	return v
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

	if _, err := uuid.Parse(claims.Subject); err != nil {
		v.logger.Debug("invalid user ID in token subject", "subject", claims.Subject)
		return nil, fmt.Errorf("invalid user ID in token subject: %w", err)
	}

	if v.expectedAudience != "" {
		if got := claims.Type(); got != v.expectedAudience {
			v.logger.Debug("token audience mismatch", "got", got, "want", v.expectedAudience)
			return nil, fmt.Errorf("token audience %q does not match expected %q", got, v.expectedAudience)
		}
	}

	v.logger.Debug("token validated", "user_id", claims.Subject, "organization_ids", claims.OrganizationIDs)
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
