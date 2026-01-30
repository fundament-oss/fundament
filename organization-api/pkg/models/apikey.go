package models

type APIKeyCreate struct {
	Name          string `validate:"required,min=1,max=255"`
	ExpiresInDays *int64 `validate:"omitempty,min=1,max=365"`
}
