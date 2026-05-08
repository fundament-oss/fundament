package dcim

import (
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func physicalConnectionFromRow(row *db.PhysicalConnectionGetByIDRow) *dcimv1.PhysicalConnection {
	conn := dcimv1.PhysicalConnection_builder{
		Id:                row.ID.String(),
		SourcePlacementId: row.APlacementID.String(),
		SourcePortName:    row.APortDefinitionID.String(),
		TargetPlacementId: row.BPlacementID.String(),
		TargetPortName:    row.BPortDefinitionID.String(),
		Created:           timestamppb.New(row.Created.Time),
	}.Build()

	if row.CableAssetID.Valid {
		conn.SetCableAssetId(uuid.UUID(row.CableAssetID.Bytes).String())
	}

	if row.LogicalConnectionID.Valid {
		conn.SetLogicalConnectionId(uuid.UUID(row.LogicalConnectionID.Bytes).String())
	}

	return conn
}

func physicalConnectionFromListRow(row *db.PhysicalConnectionListByPlacementRow) *dcimv1.PhysicalConnection {
	conn := dcimv1.PhysicalConnection_builder{
		Id:                row.ID.String(),
		SourcePlacementId: row.APlacementID.String(),
		SourcePortName:    row.APortDefinitionID.String(),
		TargetPlacementId: row.BPlacementID.String(),
		TargetPortName:    row.BPortDefinitionID.String(),
		Created:           timestamppb.New(row.Created.Time),
	}.Build()

	if row.CableAssetID.Valid {
		conn.SetCableAssetId(uuid.UUID(row.CableAssetID.Bytes).String())
	}

	if row.LogicalConnectionID.Valid {
		conn.SetLogicalConnectionId(uuid.UUID(row.LogicalConnectionID.Bytes).String())
	}

	return conn
}
