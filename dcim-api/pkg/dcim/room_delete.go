package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteRoom(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteRoomRequest],
) (*connect.Response[emptypb.Empty], error) {
	roomID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.RoomDelete(ctx, db.RoomDeleteParams{ID: roomID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete room: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("room not found"))
	}

	s.logger.InfoContext(ctx, "room deleted", "room_id", roomID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
