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

func (s *Server) CreateRoom(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateRoomRequest],
) (*connect.Response[dcimv1.CreateRoomResponse], error) {
	params := db.RoomCreateParams{
		SiteID: uuid.MustParse(req.Msg.GetSiteId()),
		Name:   req.Msg.GetName(),
	}

	if req.Msg.HasFloor() {
		params.Floor = pgtype.Text{String: req.Msg.GetFloor(), Valid: true}
	}

	id, err := s.queries.RoomCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintRoomsUqSiteName:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("room with this name already exists in this site"))
			case dbconst.ConstraintDcimRoomsFkSite:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create room: %w", err))
	}

	s.logger.InfoContext(ctx, "room created", "room_id", id)

	return connect.NewResponse(dcimv1.CreateRoomResponse_builder{
		RoomId: id.String(),
	}.Build()), nil
}
