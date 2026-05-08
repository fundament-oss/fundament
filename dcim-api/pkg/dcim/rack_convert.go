package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func rackFromGetRow(row *db.RackGetByIDRow) *dcimv1.Rack {
	return dcimv1.Rack_builder{
		Id:            row.ID.String(),
		RowId:         row.RackRowID.String(),
		Name:          row.Name,
		TotalUnits:    row.TotalUnits,
		PositionInRow: row.PositionInRow,
		Created:       timestamppb.New(row.Created.Time),
	}.Build()
}

func rackFromListRow(row *db.RackListRow) *dcimv1.Rack {
	return dcimv1.Rack_builder{
		Id:            row.ID.String(),
		RowId:         row.RackRowID.String(),
		Name:          row.Name,
		TotalUnits:    row.TotalUnits,
		PositionInRow: row.PositionInRow,
		Created:       timestamppb.New(row.Created.Time),
	}.Build()
}
