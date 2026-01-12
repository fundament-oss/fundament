package models

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
