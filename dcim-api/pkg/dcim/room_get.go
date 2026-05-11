package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetRoom(
	ctx context.Context,
	req *connect.Request[dcimv1.GetRoomRequest],
) (*connect.Response[dcimv1.GetRoomResponse], error) {
	roomID := uuid.MustParse(req.Msg.GetId())

	room, err := s.queries.RoomGetByID(ctx, db.RoomGetByIDParams{ID: roomID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("room not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get room: %w", err))
	}

	return connect.NewResponse(dcimv1.GetRoomResponse_builder{
		Room: roomFromRow(&room),
	}.Build()), nil
}
