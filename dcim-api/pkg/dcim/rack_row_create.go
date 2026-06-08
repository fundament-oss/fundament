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

func (s *Server) CreateRackRow(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateRackRowRequest],
) (*connect.Response[dcimv1.CreateRackRowResponse], error) {
	params := db.RackRowCreateParams{
		RoomID: uuid.MustParse(req.Msg.GetRoomId()),
		Name:   req.Msg.GetName(),
	}

	if req.Msg.HasPositionX() {
		params.PositionX = pgtype.Float8{Float64: req.Msg.GetPositionX(), Valid: true}
	}

	if req.Msg.HasPositionY() {
		params.PositionY = pgtype.Float8{Float64: req.Msg.GetPositionY(), Valid: true}
	}

	id, err := s.queries.RackRowCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintRackRowsUqRoomName:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("rack row with this name already exists in this room"))
			case dbconst.ConstraintDcimRackRowsFkRoom:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("room not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create rack row: %w", err))
	}

	s.logger.InfoContext(ctx, "rack row created", "rack_row_id", id)

	return connect.NewResponse(dcimv1.CreateRackRowResponse_builder{
		RackRowId: id.String(),
	}.Build()), nil
}
