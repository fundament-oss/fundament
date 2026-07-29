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

func (s *Server) UpdateRack(
	ctx context.Context,
	req *dcimv1.UpdateRackRequest,
) (*emptypb.Empty, error) {
	rackID := uuid.MustParse(req.GetId())

	params := db.RackUpdateParams{
		ID: rackID,
	}

	if req.HasName() {
		params.Name = pgtype.Text{String: req.GetName(), Valid: true}
	}

	if req.HasTotalUnits() {
		params.TotalUnits = pgtype.Int4{Int32: req.GetTotalUnits(), Valid: true}
	}

	if req.HasPositionInRow() {
		params.PositionInRow = pgtype.Int4{Int32: req.GetPositionInRow(), Valid: true}
	}

	rowsAffected, err := s.queries.RackUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintRacksUqRackRowName {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("rack with this name already exists in this rack row"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update rack: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack not found"))
	}

	s.logger.InfoContext(ctx, "rack updated", "rack_id", rackID)

	return &emptypb.Empty{}, nil
}
