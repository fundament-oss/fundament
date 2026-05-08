package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func logicalConnectionTypeToProto(s string) dcimv1.LogicalConnectionType {
	switch s {
	case "network":
		return dcimv1.LogicalConnectionType_LOGICAL_CONNECTION_TYPE_NETWORK
	case "power":
		return dcimv1.LogicalConnectionType_LOGICAL_CONNECTION_TYPE_POWER
	case "console":
		return dcimv1.LogicalConnectionType_LOGICAL_CONNECTION_TYPE_CONSOLE
	default:
		panic("unhandled logical connection type: " + s)
	}
}

func logicalConnectionTypeToDB(t dcimv1.LogicalConnectionType) string {
	switch t {
	case dcimv1.LogicalConnectionType_LOGICAL_CONNECTION_TYPE_NETWORK:
		return "network"
	case dcimv1.LogicalConnectionType_LOGICAL_CONNECTION_TYPE_POWER:
		return "power"
	case dcimv1.LogicalConnectionType_LOGICAL_CONNECTION_TYPE_CONSOLE:
		return "console"
	default:
		panic("unhandled logical connection type enum")
	}
}

func logicalConnectionFromRow(row *db.LogicalConnectionGetByIDRow) *dcimv1.LogicalConnection {
	conn := dcimv1.LogicalConnection_builder{
		Id:             row.ID.String(),
		DesignId:       row.LogicalDesignID.String(),
		SourceDeviceId: row.ALogicalDeviceID.String(),
		TargetDeviceId: row.BLogicalDeviceID.String(),
		ConnectionType: logicalConnectionTypeToProto(row.ConnectionType),
		Label:          row.Label.String,
		Created:        timestamppb.New(row.Created.Time),
	}.Build()

	if row.APortRole.Valid {
		conn.SetSourcePortRole(row.APortRole.String)
	}

	if row.BPortRole.Valid {
		conn.SetTargetPortRole(row.BPortRole.String)
	}

	if row.Requirements.Valid {
		conn.SetRequirements(row.Requirements.String)
	}

	return conn
}

func logicalConnectionFromListRow(row *db.LogicalConnectionListRow) *dcimv1.LogicalConnection {
	conn := dcimv1.LogicalConnection_builder{
		Id:             row.ID.String(),
		DesignId:       row.LogicalDesignID.String(),
		SourceDeviceId: row.ALogicalDeviceID.String(),
		TargetDeviceId: row.BLogicalDeviceID.String(),
		ConnectionType: logicalConnectionTypeToProto(row.ConnectionType),
		Label:          row.Label.String,
		Created:        timestamppb.New(row.Created.Time),
	}.Build()

	if row.APortRole.Valid {
		conn.SetSourcePortRole(row.APortRole.String)
	}

	if row.BPortRole.Valid {
		conn.SetTargetPortRole(row.BPortRole.String)
	}

	if row.Requirements.Valid {
		conn.SetRequirements(row.Requirements.String)
	}

	return conn
}
