package main

import (
	"fmt"
	"net/http"

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

func (s *OrganizationServer) validateRequest(header http.Header) (*Claims, error) {
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
