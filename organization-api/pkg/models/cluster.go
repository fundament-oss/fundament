package models

import "github.com/google/uuid"

type ClusterGet struct {
	ClusterID uuid.UUID `validate:"required"`
}

type ClusterCreate struct {
	Name              string `validate:"required,min=1,max=255"`
	Region            string `validate:"required"`
	KubernetesVersion string `validate:"required"`
}

type ClusterUpdate struct {
	ClusterID         uuid.UUID `validate:"required"`
	KubernetesVersion *string   `validate:"omitempty"`
}
