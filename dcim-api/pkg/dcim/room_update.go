package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateRoom(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdateRoomRequest],
) (*connect.Response[dcimv1.UpdateRoomResponse], error) {
	roomID := uuid.MustParse(req.Msg.GetId())

	params := db.RoomUpdateParams{
		ID: roomID,
	}

	if req.Msg.HasName() {
		params.Name = pgtype.Text{String: req.Msg.GetName(), Valid: true}
	}

	if req.Msg.HasFloor() {
		params.Floor = pgtype.Text{String: req.Msg.GetFloor(), Valid: true}
	}

	rowsAffected, err := s.queries.RoomUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintRoomsUqSiteName {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("room with this name already exists in this site"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update room: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("room not found"))
	}

	s.logger.InfoContext(ctx, "room updated", "room_id", roomID)

	return connect.NewResponse(dcimv1.UpdateRoomResponse_builder{}.Build()), nil
}
