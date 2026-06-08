package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func logicalDesignStatusToProto(s string) dcimv1.LogicalDesignStatus {
	switch s {
	case "draft":
		return dcimv1.LogicalDesignStatus_LOGICAL_DESIGN_STATUS_DRAFT
	case "active":
		return dcimv1.LogicalDesignStatus_LOGICAL_DESIGN_STATUS_ACTIVE
	case "archived":
		return dcimv1.LogicalDesignStatus_LOGICAL_DESIGN_STATUS_ARCHIVED
	default:
		panic("unhandled logical design status: " + s)
	}
}

func logicalDesignStatusToDB(s dcimv1.LogicalDesignStatus) string {
	switch s {
	case dcimv1.LogicalDesignStatus_LOGICAL_DESIGN_STATUS_DRAFT:
		return "draft"
	case dcimv1.LogicalDesignStatus_LOGICAL_DESIGN_STATUS_ACTIVE:
		return "active"
	case dcimv1.LogicalDesignStatus_LOGICAL_DESIGN_STATUS_ARCHIVED:
		return "archived"
	default:
		panic("unhandled logical design status enum")
	}
}

func designFromRow(row *db.LogicalDesignGetByIDRow) *dcimv1.LogicalDesign {
	design := dcimv1.LogicalDesign_builder{
		Id:      row.ID.String(),
		Name:    row.Name,
		Version: row.Version,
		Status:  logicalDesignStatusToProto(row.Status),
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.Description.Valid {
		design.SetDescription(row.Description.String)
	}

	return design
}

func designFromListRow(row *db.LogicalDesignListRow) *dcimv1.LogicalDesign {
	design := dcimv1.LogicalDesign_builder{
		Id:      row.ID.String(),
		Name:    row.Name,
		Version: row.Version,
		Status:  logicalDesignStatusToProto(row.Status),
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.Description.Valid {
		design.SetDescription(row.Description.String)
	}

	return design
}
