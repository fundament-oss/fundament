package dcim

import (
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func rackSlotTypeToProto(s string) dcimv1.RackSlotType {
	switch s {
	case "unit":
		return dcimv1.RackSlotType_RACK_SLOT_TYPE_UNIT
	case "power":
		return dcimv1.RackSlotType_RACK_SLOT_TYPE_POWER
	case "zero_u":
		return dcimv1.RackSlotType_RACK_SLOT_TYPE_ZERO_U
	default:
		panic("unhandled rack slot type: " + s)
	}
}

func rackSlotTypeToDB(t dcimv1.RackSlotType) string {
	switch t {
	case dcimv1.RackSlotType_RACK_SLOT_TYPE_UNIT:
		return "unit"
	case dcimv1.RackSlotType_RACK_SLOT_TYPE_POWER:
		return "power"
	case dcimv1.RackSlotType_RACK_SLOT_TYPE_ZERO_U:
		return "zero_u"
	default:
		panic("unhandled rack slot type enum")
	}
}

func placementFromGetRow(row *db.PlacementGetByIDRow) *dcimv1.Placement {
	p := dcimv1.Placement_builder{
		Id:      row.ID.String(),
		AssetId: row.AssetID.String(),
		Notes:   row.Notes.String,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.RackID.Valid {
		p.SetRack(dcimv1.RackLocation_builder{
			RackId:        uuid.UUID(row.RackID.Bytes).String(),
			RackUnitStart: row.StartUnit.Int32,
			RackSlotType:  rackSlotTypeToProto(row.SlotType.String),
		}.Build())
	} else if row.ParentPlacementID.Valid {
		p.SetSubComponent(dcimv1.SubComponentLocation_builder{
			ParentPlacementId: uuid.UUID(row.ParentPlacementID.Bytes).String(),
			ParentPortName:    uuid.UUID(row.PortDefinitionID.Bytes).String(),
		}.Build())
	}

	if row.LogicalDeviceID.Valid {
		p.SetLogicalDeviceId(uuid.UUID(row.LogicalDeviceID.Bytes).String())
	}

	if row.ExternalRef.Valid {
		p.SetExternalRef(row.ExternalRef.String)
	}

	return p
}

func placementFromRackListRow(row *db.PlacementListByRackRow) *dcimv1.Placement {
	p := dcimv1.Placement_builder{
		Id:      row.ID.String(),
		AssetId: row.AssetID.String(),
		Notes:   row.Notes.String,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.RackID.Valid {
		p.SetRack(dcimv1.RackLocation_builder{
			RackId:        uuid.UUID(row.RackID.Bytes).String(),
			RackUnitStart: row.StartUnit.Int32,
			RackSlotType:  rackSlotTypeToProto(row.SlotType.String),
		}.Build())
	}

	if row.LogicalDeviceID.Valid {
		p.SetLogicalDeviceId(uuid.UUID(row.LogicalDeviceID.Bytes).String())
	}

	if row.ExternalRef.Valid {
		p.SetExternalRef(row.ExternalRef.String)
	}

	return p
}

func placementFromParentListRow(row *db.PlacementListByParentRow) *dcimv1.Placement {
	p := dcimv1.Placement_builder{
		Id:      row.ID.String(),
		AssetId: row.AssetID.String(),
		Notes:   row.Notes.String,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.ParentPlacementID.Valid {
		p.SetSubComponent(dcimv1.SubComponentLocation_builder{
			ParentPlacementId: uuid.UUID(row.ParentPlacementID.Bytes).String(),
			ParentPortName:    uuid.UUID(row.PortDefinitionID.Bytes).String(),
		}.Build())
	}

	if row.LogicalDeviceID.Valid {
		p.SetLogicalDeviceId(uuid.UUID(row.LogicalDeviceID.Bytes).String())
	}

	if row.ExternalRef.Valid {
		p.SetExternalRef(row.ExternalRef.String)
	}

	return p
}
