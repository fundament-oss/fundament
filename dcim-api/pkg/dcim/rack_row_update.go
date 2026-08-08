package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateRackRow(
	ctx context.Context,
	req *dcimv1.UpdateRackRowRequest,
) (*emptypb.Empty, error) {
	rackRowID := uuid.MustParse(req.GetId())

	params := db.RackRowUpdateParams{
		ID: rackRowID,
	}

	if req.HasName() {
		params.Name = pgtype.Text{String: req.GetName(), Valid: true}
	}

	if req.HasPositionX() {
		params.PositionX = pgtype.Float8{Float64: req.GetPositionX(), Valid: true}
	}

	if req.HasPositionY() {
		params.PositionY = pgtype.Float8{Float64: req.GetPositionY(), Valid: true}
	}

	rowsAffected, err := s.queries.RackRowUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintRackRowsUqRoomName {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("rack row with this name already exists in this room"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update rack row: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack row not found"))
	}

	s.logger.InfoContext(ctx, "rack row updated", "rack_row_id", rackRowID)

	return &emptypb.Empty{}, nil
}
