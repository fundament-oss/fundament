package organization

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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

// verifyCookieValue verifies a signed cookie value and returns the original value
// Format is: <value>.<signature> where value may contain dots (e.g., JWT)
func (s *OrganizationServer) verifyCookieValue(signedValue string) (string, error) {
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
		tokenString, err = s.verifyCookieValue(tokenValue)
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
