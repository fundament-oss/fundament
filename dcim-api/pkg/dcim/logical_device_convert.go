package dcim

import (
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func logicalDeviceRoleToProto(s string) dcimv1.LogicalDeviceRole {
	switch s {
	case "compute":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_COMPUTE
	case "tor":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_TOR
	case "spine":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_SPINE
	case "core":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_CORE
	case "pdu":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_PDU
	case "patch_panel":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_PATCH_PANEL
	case "storage":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_STORAGE
	case "firewall":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_FIREWALL
	case "load_balancer":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_LOAD_BALANCER
	case "console_server":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_CONSOLE_SERVER
	case "cable_manager":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_CABLE_MANAGER
	case "adapter":
		return dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_ADAPTER
	default:
		panic("unhandled logical device role: " + s)
	}
}

func logicalDeviceRoleToDB(r dcimv1.LogicalDeviceRole) string {
	switch r {
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_COMPUTE:
		return "compute"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_TOR:
		return "tor"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_SPINE:
		return "spine"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_CORE:
		return "core"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_PDU:
		return "pdu"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_PATCH_PANEL:
		return "patch_panel"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_STORAGE:
		return "storage"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_FIREWALL:
		return "firewall"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_LOAD_BALANCER:
		return "load_balancer"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_CONSOLE_SERVER:
		return "console_server"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_CABLE_MANAGER:
		return "cable_manager"
	case dcimv1.LogicalDeviceRole_LOGICAL_DEVICE_ROLE_ADAPTER:
		return "adapter"
	default:
		panic("unhandled logical device role enum")
	}
}

func logicalDeviceFromRow(row *db.LogicalDeviceGetByIDRow) *dcimv1.LogicalDevice {
	device := dcimv1.LogicalDevice_builder{
		Id:       row.ID.String(),
		DesignId: row.LogicalDesignID.String(),
		Label:    row.Label,
		Role:     logicalDeviceRoleToProto(row.Role),
		Notes:    row.Notes.String,
		Created:  timestamppb.New(row.Created.Time),
	}.Build()

	if row.DeviceCatalogID.Valid {
		device.SetDeviceCatalogId(uuid.UUID(row.DeviceCatalogID.Bytes).String())
	}

	if row.Requirements.Valid {
		device.SetRequirements(row.Requirements.String)
	}

	return device
}

func logicalDeviceFromListRow(row *db.LogicalDeviceListRow) *dcimv1.LogicalDevice {
	device := dcimv1.LogicalDevice_builder{
		Id:       row.ID.String(),
		DesignId: row.LogicalDesignID.String(),
		Label:    row.Label,
		Role:     logicalDeviceRoleToProto(row.Role),
		Notes:    row.Notes.String,
		Created:  timestamppb.New(row.Created.Time),
	}.Build()

	if row.DeviceCatalogID.Valid {
		device.SetDeviceCatalogId(uuid.UUID(row.DeviceCatalogID.Bytes).String())
	}

	if row.Requirements.Valid {
		device.SetRequirements(row.Requirements.String)
	}

	return device
}
