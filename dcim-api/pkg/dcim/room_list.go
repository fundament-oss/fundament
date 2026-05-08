package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListRooms(
	ctx context.Context,
	req *connect.Request[dcimv1.ListRoomsRequest],
) (*connect.Response[dcimv1.ListRoomsResponse], error) {
	params := db.RoomListParams{}

	if req.Msg.HasSiteId() {
		siteID := uuid.MustParse(req.Msg.GetSiteId())
		params.SiteID = pgtype.UUID{Bytes: siteID, Valid: true}
	}

	rows, err := s.queries.RoomList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list rooms: %w", err))
	}

	rooms := make([]*dcimv1.Room, 0, len(rows))
	for _, row := range rows {
		rooms = append(rooms, roomFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListRoomsResponse_builder{
		Rooms: rooms,
	}.Build()), nil
}
