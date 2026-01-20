package model

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// User represents user data for JWT generation.
type User struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	ExternalID     string
}

type Claims struct {
	jwt.RegisteredClaims
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Groups         []string  `json:"groups"`
	Name           string    `json:"name"`
}
