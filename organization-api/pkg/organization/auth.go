package organization

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
)

// Claims represents the JWT claims from the authn-api.

type Claims struct {
	jwt.RegisteredClaims
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Groups   []string  `json:"groups"`
	Name     string    `json:"name"`
}

const AuthCookieName = "fundament_auth"

// validateRequest validates auth from either Authorization header or Cookie header
func (s *OrganizationServer) validateRequest(header http.Header) (*Claims, error) {
	var tokenString string

	// First try Authorization header
	authHeader := header.Get("Authorization")
	if len(authHeader) >= 8 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	} else {
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
		var err error
		tokenString, err = auth.VerifyCookieValue(tokenValue, s.config.JWTSecret)
		if err != nil {
			return nil, fmt.Errorf("invalid cookie signature: %w", err)
		}
	}

	// Parse and validate JWT
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
