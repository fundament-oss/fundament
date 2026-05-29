package dcim

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func cableTypeToProto(s pgtype.Text) dcimv1.CableType {
	if !s.Valid {
		return dcimv1.CableType_CABLE_TYPE_UNSPECIFIED
	}
	switch dbconst.PhysicalConnectionCableType(s.String) {
	case dbconst.PhysicalConnectionCableType_Cat5e:
		return dcimv1.CableType_CABLE_TYPE_CAT5E
	case dbconst.PhysicalConnectionCableType_Cat6:
		return dcimv1.CableType_CABLE_TYPE_CAT6
	case dbconst.PhysicalConnectionCableType_Cat6a:
		return dcimv1.CableType_CABLE_TYPE_CAT6A
	case dbconst.PhysicalConnectionCableType_Cat7:
		return dcimv1.CableType_CABLE_TYPE_CAT7
	case dbconst.PhysicalConnectionCableType_Cat8:
		return dcimv1.CableType_CABLE_TYPE_CAT8
	case dbconst.PhysicalConnectionCableType_Dac:
		return dcimv1.CableType_CABLE_TYPE_DAC
	case dbconst.PhysicalConnectionCableType_Aoc:
		return dcimv1.CableType_CABLE_TYPE_AOC
	case dbconst.PhysicalConnectionCableType_Mmf:
		return dcimv1.CableType_CABLE_TYPE_MMF
	case dbconst.PhysicalConnectionCableType_Smf:
		return dcimv1.CableType_CABLE_TYPE_SMF
	case dbconst.PhysicalConnectionCableType_Power:
		return dcimv1.CableType_CABLE_TYPE_POWER
	case dbconst.PhysicalConnectionCableType_Console:
		return dcimv1.CableType_CABLE_TYPE_CONSOLE
	case dbconst.PhysicalConnectionCableType_Usb:
		return dcimv1.CableType_CABLE_TYPE_USB
	case dbconst.PhysicalConnectionCableType_Other:
		return dcimv1.CableType_CABLE_TYPE_OTHER
	default:
		panic("unhandled physical connection cable_type: " + s.String)
	}
}

func cableTypeToDB(t dcimv1.CableType) pgtype.Text {
	switch t {
	case dcimv1.CableType_CABLE_TYPE_UNSPECIFIED:
		return pgtype.Text{}
	case dcimv1.CableType_CABLE_TYPE_CAT5E:
		return dbText(dbconst.PhysicalConnectionCableType_Cat5e)
	case dcimv1.CableType_CABLE_TYPE_CAT6:
		return dbText(dbconst.PhysicalConnectionCableType_Cat6)
	case dcimv1.CableType_CABLE_TYPE_CAT6A:
		return dbText(dbconst.PhysicalConnectionCableType_Cat6a)
	case dcimv1.CableType_CABLE_TYPE_CAT7:
		return dbText(dbconst.PhysicalConnectionCableType_Cat7)
	case dcimv1.CableType_CABLE_TYPE_CAT8:
		return dbText(dbconst.PhysicalConnectionCableType_Cat8)
	case dcimv1.CableType_CABLE_TYPE_DAC:
		return dbText(dbconst.PhysicalConnectionCableType_Dac)
	case dcimv1.CableType_CABLE_TYPE_AOC:
		return dbText(dbconst.PhysicalConnectionCableType_Aoc)
	case dcimv1.CableType_CABLE_TYPE_MMF:
		return dbText(dbconst.PhysicalConnectionCableType_Mmf)
	case dcimv1.CableType_CABLE_TYPE_SMF:
		return dbText(dbconst.PhysicalConnectionCableType_Smf)
	case dcimv1.CableType_CABLE_TYPE_POWER:
		return dbText(dbconst.PhysicalConnectionCableType_Power)
	case dcimv1.CableType_CABLE_TYPE_CONSOLE:
		return dbText(dbconst.PhysicalConnectionCableType_Console)
	case dcimv1.CableType_CABLE_TYPE_USB:
		return dbText(dbconst.PhysicalConnectionCableType_Usb)
	case dcimv1.CableType_CABLE_TYPE_OTHER:
		return dbText(dbconst.PhysicalConnectionCableType_Other)
	default:
		panic("unhandled cable type enum: " + t.String())
	}
}

func cableStatusToProto(s pgtype.Text) dcimv1.CableStatus {
	if !s.Valid {
		return dcimv1.CableStatus_CABLE_STATUS_UNSPECIFIED
	}
	switch dbconst.PhysicalConnectionStatus(s.String) {
	case dbconst.PhysicalConnectionStatus_Planned:
		return dcimv1.CableStatus_CABLE_STATUS_PLANNED
	case dbconst.PhysicalConnectionStatus_Connected:
		return dcimv1.CableStatus_CABLE_STATUS_CONNECTED
	case dbconst.PhysicalConnectionStatus_Decommissioned:
		return dcimv1.CableStatus_CABLE_STATUS_DECOMMISSIONED
	default:
		panic("unhandled physical connection status: " + s.String)
	}
}

func cableStatusToDB(s dcimv1.CableStatus) pgtype.Text {
	switch s {
	case dcimv1.CableStatus_CABLE_STATUS_UNSPECIFIED:
		return pgtype.Text{}
	case dcimv1.CableStatus_CABLE_STATUS_PLANNED:
		return dbText(dbconst.PhysicalConnectionStatus_Planned)
	case dcimv1.CableStatus_CABLE_STATUS_CONNECTED:
		return dbText(dbconst.PhysicalConnectionStatus_Connected)
	case dcimv1.CableStatus_CABLE_STATUS_DECOMMISSIONED:
		return dbText(dbconst.PhysicalConnectionStatus_Decommissioned)
	default:
		panic("unhandled cable status enum: " + s.String())
	}
}

func cableColorToProto(s pgtype.Text) dcimv1.CableColor {
	if !s.Valid {
		return dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED
	}
	switch dbconst.PhysicalConnectionColor(s.String) {
	case dbconst.PhysicalConnectionColor_DarkGrey:
		return dcimv1.CableColor_CABLE_COLOR_DARK_GREY
	case dbconst.PhysicalConnectionColor_LightGrey:
		return dcimv1.CableColor_CABLE_COLOR_LIGHT_GREY
	case dbconst.PhysicalConnectionColor_Red:
		return dcimv1.CableColor_CABLE_COLOR_RED
	case dbconst.PhysicalConnectionColor_Green:
		return dcimv1.CableColor_CABLE_COLOR_GREEN
	case dbconst.PhysicalConnectionColor_Blue:
		return dcimv1.CableColor_CABLE_COLOR_BLUE
	case dbconst.PhysicalConnectionColor_Yellow:
		return dcimv1.CableColor_CABLE_COLOR_YELLOW
	case dbconst.PhysicalConnectionColor_Purple:
		return dcimv1.CableColor_CABLE_COLOR_PURPLE
	case dbconst.PhysicalConnectionColor_Orange:
		return dcimv1.CableColor_CABLE_COLOR_ORANGE
	case dbconst.PhysicalConnectionColor_Teal:
		return dcimv1.CableColor_CABLE_COLOR_TEAL
	case dbconst.PhysicalConnectionColor_White:
		return dcimv1.CableColor_CABLE_COLOR_WHITE
	default:
		panic("unhandled physical connection color: " + s.String)
	}
}

func cableColorToDB(c dcimv1.CableColor) pgtype.Text {
	switch c {
	case dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED:
		return pgtype.Text{}
	case dcimv1.CableColor_CABLE_COLOR_DARK_GREY:
		return dbText(dbconst.PhysicalConnectionColor_DarkGrey)
	case dcimv1.CableColor_CABLE_COLOR_LIGHT_GREY:
		return dbText(dbconst.PhysicalConnectionColor_LightGrey)
	case dcimv1.CableColor_CABLE_COLOR_RED:
		return dbText(dbconst.PhysicalConnectionColor_Red)
	case dcimv1.CableColor_CABLE_COLOR_GREEN:
		return dbText(dbconst.PhysicalConnectionColor_Green)
	case dcimv1.CableColor_CABLE_COLOR_BLUE:
		return dbText(dbconst.PhysicalConnectionColor_Blue)
	case dcimv1.CableColor_CABLE_COLOR_YELLOW:
		return dbText(dbconst.PhysicalConnectionColor_Yellow)
	case dcimv1.CableColor_CABLE_COLOR_PURPLE:
		return dbText(dbconst.PhysicalConnectionColor_Purple)
	case dcimv1.CableColor_CABLE_COLOR_ORANGE:
		return dbText(dbconst.PhysicalConnectionColor_Orange)
	case dcimv1.CableColor_CABLE_COLOR_TEAL:
		return dbText(dbconst.PhysicalConnectionColor_Teal)
	case dcimv1.CableColor_CABLE_COLOR_WHITE:
		return dbText(dbconst.PhysicalConnectionColor_White)
	default:
		panic("unhandled cable color enum: " + c.String())
	}
}

// dbText wraps a dbconst enum value as a non-null pgtype.Text.
func dbText[T ~string](v T) pgtype.Text {
	return pgtype.Text{String: string(v), Valid: true}
}

func physicalConnectionFromFields(
	id uuid.UUID,
	aPlacementID, aPortDefinitionID, bPlacementID, bPortDefinitionID uuid.UUID,
	cableAssetID, logicalConnectionID pgtype.UUID,
	cableType, status, color, label pgtype.Text,
	created pgtype.Timestamptz,
) *dcimv1.PhysicalConnection {
	conn := dcimv1.PhysicalConnection_builder{
		Id:                     id.String(),
		SourcePlacementId:      aPlacementID.String(),
		SourcePortDefinitionId: aPortDefinitionID.String(),
		TargetPlacementId:      bPlacementID.String(),
		TargetPortDefinitionId: bPortDefinitionID.String(),
		CableType:              cableTypeToProto(cableType),
		Status:                 cableStatusToProto(status),
		Color:                  cableColorToProto(color),
		Label:                  label.String,
		Created:                timestamppb.New(created.Time),
	}.Build()

	if cableAssetID.Valid {
		conn.SetCableAssetId(uuid.UUID(cableAssetID.Bytes).String())
	}

	if logicalConnectionID.Valid {
		conn.SetLogicalConnectionId(uuid.UUID(logicalConnectionID.Bytes).String())
	}

	return conn
}

func physicalConnectionFromRow(row *db.PhysicalConnectionGetByIDRow) *dcimv1.PhysicalConnection {
	return physicalConnectionFromFields(
		row.ID, row.APlacementID, row.APortDefinitionID, row.BPlacementID, row.BPortDefinitionID,
		row.CableAssetID, row.LogicalConnectionID,
		row.CableType, row.Status, row.Color, row.Label,
		row.Created,
	)
}

func physicalConnectionFromListRow(row *db.PhysicalConnectionListByPlacementRow) *dcimv1.PhysicalConnection {
	return physicalConnectionFromFields(
		row.ID, row.APlacementID, row.APortDefinitionID, row.BPlacementID, row.BPortDefinitionID,
		row.CableAssetID, row.LogicalConnectionID,
		row.CableType, row.Status, row.Color, row.Label,
		row.Created,
	)
}
