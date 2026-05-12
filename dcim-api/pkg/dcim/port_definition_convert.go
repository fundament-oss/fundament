package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func portTypeToProto(pt string) dcimv1.PortType {
	switch pt {
	case "network":
		return dcimv1.PortType_PORT_TYPE_NETWORK
	case "power_in":
		return dcimv1.PortType_PORT_TYPE_POWER_IN
	case "power_out":
		return dcimv1.PortType_PORT_TYPE_POWER_OUT
	case "slot":
		return dcimv1.PortType_PORT_TYPE_SLOT
	case "bay":
		return dcimv1.PortType_PORT_TYPE_BAY
	case "console":
		return dcimv1.PortType_PORT_TYPE_CONSOLE
	default:
		panic("unhandled port type: " + pt)
	}
}

func portTypeToDB(pt dcimv1.PortType) string {
	switch pt {
	case dcimv1.PortType_PORT_TYPE_NETWORK:
		return "network"
	case dcimv1.PortType_PORT_TYPE_POWER_IN:
		return "power_in"
	case dcimv1.PortType_PORT_TYPE_POWER_OUT:
		return "power_out"
	case dcimv1.PortType_PORT_TYPE_SLOT:
		return "slot"
	case dcimv1.PortType_PORT_TYPE_BAY:
		return "bay"
	case dcimv1.PortType_PORT_TYPE_CONSOLE:
		return "console"
	default:
		panic("unhandled port type enum: " + pt.String())
	}
}

func portDirectionToProto(d string) dcimv1.PortDirection {
	switch d {
	case "in":
		return dcimv1.PortDirection_PORT_DIRECTION_IN
	case "out":
		return dcimv1.PortDirection_PORT_DIRECTION_OUT
	case "bidir":
		return dcimv1.PortDirection_PORT_DIRECTION_BIDIR
	default:
		panic("unhandled port direction: " + d)
	}
}

func portDirectionToDB(d dcimv1.PortDirection) string {
	switch d {
	case dcimv1.PortDirection_PORT_DIRECTION_IN:
		return "in"
	case dcimv1.PortDirection_PORT_DIRECTION_OUT:
		return "out"
	case dcimv1.PortDirection_PORT_DIRECTION_BIDIR:
		return "bidir"
	default:
		panic("unhandled port direction enum: " + d.String())
	}
}

func portDefinitionFromGetRow(row *db.PortDefinitionGetByIDRow) *dcimv1.PortDefinition {
	pd := dcimv1.PortDefinition_builder{
		Id:              row.ID.String(),
		DeviceCatalogId: row.DeviceCatalogID.String(),
		Name:            row.Name,
		PortType:        portTypeToProto(row.PortType),
		Direction:       portDirectionToProto(row.Direction),
		Ordinal:         row.Ordinal,
	}.Build()

	if row.MediaType.Valid {
		pd.SetMediaType(row.MediaType.String)
	}

	if row.Speed.Valid {
		pd.SetSpeed(row.Speed.String)
	}

	if row.MaxPowerW.Valid {
		pd.SetMaxPowerW(numericToFloat64(row.MaxPowerW))
	}

	return pd
}

func portDefinitionFromListRow(row *db.PortDefinitionListRow) *dcimv1.PortDefinition {
	pd := dcimv1.PortDefinition_builder{
		Id:              row.ID.String(),
		DeviceCatalogId: row.DeviceCatalogID.String(),
		Name:            row.Name,
		PortType:        portTypeToProto(row.PortType),
		Direction:       portDirectionToProto(row.Direction),
		Ordinal:         row.Ordinal,
	}.Build()

	if row.MediaType.Valid {
		pd.SetMediaType(row.MediaType.String)
	}

	if row.Speed.Valid {
		pd.SetSpeed(row.Speed.String)
	}

	if row.MaxPowerW.Valid {
		pd.SetMaxPowerW(numericToFloat64(row.MaxPowerW))
	}

	return pd
}
