package adapter

import (
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func ToNodePoolCreate(req *organizationv1.NodePoolSpec) models.NodePoolCreate {
	return models.NodePoolCreate{
		Name:         req.Name,
		MachineType:  req.MachineType,
		AutoscaleMin: req.AutoscaleMin,
		AutoscaleMax: req.AutoscaleMax,
	}
}

func ToNodePoolUpdate(req *organizationv1.UpdateNodePoolRequest) models.NodePoolUpdate {
	return models.NodePoolUpdate{
		AutoscaleMin: req.AutoscaleMin,
		AutoscaleMax: req.AutoscaleMax,
	}
}
