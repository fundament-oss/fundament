package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func rackRowFromRow(row *db.RackRowGetByIDRow) *dcimv1.RackRow {
	rr := dcimv1.RackRow_builder{
		Id:      row.ID.String(),
		RoomId:  row.RoomID.String(),
		Name:    row.Name,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.PositionX.Valid {
		rr.SetPositionX(row.PositionX.Float64)
	}

	if row.PositionY.Valid {
		rr.SetPositionY(row.PositionY.Float64)
	}

	return rr
}

func rackRowFromListRow(row *db.RackRowListRow) *dcimv1.RackRow {
	rr := dcimv1.RackRow_builder{
		Id:      row.ID.String(),
		RoomId:  row.RoomID.String(),
		Name:    row.Name,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.PositionX.Valid {
		rr.SetPositionX(row.PositionX.Float64)
	}

	if row.PositionY.Valid {
		rr.SetPositionY(row.PositionY.Float64)
	}

	return rr
}
