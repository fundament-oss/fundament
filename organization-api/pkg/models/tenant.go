package models

import "github.com/google/uuid"

type OrganizationUpdate struct {
	ID   uuid.UUID `validate:"required"`
	Name string    `validate:"required,min=1,max=255"`
}
