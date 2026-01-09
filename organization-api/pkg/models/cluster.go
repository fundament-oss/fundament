package models

import "github.com/google/uuid"

type ClusterGet struct {
	ClusterID uuid.UUID `validate:"required"`
}

type ClusterCreate struct {
	Name              string           `validate:"required,min=1,max=255"`
	Region            string           `validate:"required"`
	KubernetesVersion string           `validate:"required"`
	NodePools         []NodePoolCreate `validate:"dive"`
}

type ClusterUpdate struct {
	ClusterID         uuid.UUID `validate:"required"`
	KubernetesVersion *string   `validate:"omitempty"`
}

type NodePoolCreate struct {
	Name         string `validate:"required,min=1,max=255"`
	MachineType  string `validate:"required"`
	AutoscaleMin int32  `validate:"required,gte=0"`
	AutoscaleMax int32  `validate:"required,gtefield=AutoscaleMin"`
}
type NodePoolUpdate struct {
	AutoscaleMin int32 `validate:"required,gte=0"`
	AutoscaleMax int32 `validate:"required,gtefield=AutoscaleMin"`
}
