package models

import "github.com/google/uuid"

// Project member roles
const (
	ProjectRoleAdmin  = "admin"
	ProjectRoleViewer = "viewer"
)

type ProjectCreate struct {
	Name string `validate:"required,min=1,max=255"`
}

type ProjectUpdate struct {
	ProjectID uuid.UUID `validate:"required"`
	Name      *string   `validate:"omitempty,min=1,max=255"`
}
