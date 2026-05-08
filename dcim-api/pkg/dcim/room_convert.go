package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func roomFromRow(row *db.RoomGetByIDRow) *dcimv1.Room {
	room := dcimv1.Room_builder{
		Id:      row.ID.String(),
		SiteId:  row.SiteID.String(),
		Name:    row.Name,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.Floor.Valid {
		room.SetFloor(row.Floor.String)
	}

	return room
}

func roomFromListRow(row *db.RoomListRow) *dcimv1.Room {
	room := dcimv1.Room_builder{
		Id:      row.ID.String(),
		SiteId:  row.SiteID.String(),
		Name:    row.Name,
		Created: timestamppb.New(row.Created.Time),
	}.Build()

	if row.Floor.Valid {
		room.SetFloor(row.Floor.String)
	}

	return room
}
