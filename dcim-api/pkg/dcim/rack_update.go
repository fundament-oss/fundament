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
	req *connect.Request[dcimv1.UpdateRackRequest],
) (*connect.Response[emptypb.Empty], error) {
	rackID := uuid.MustParse(req.Msg.GetId())

	params := db.RackUpdateParams{
		ID: rackID,
	}

	if req.Msg.HasName() {
		params.Name = pgtype.Text{String: req.Msg.GetName(), Valid: true}
	}

	if req.Msg.HasTotalUnits() {
		params.TotalUnits = pgtype.Int4{Int32: req.Msg.GetTotalUnits(), Valid: true}
	}

	if req.Msg.HasPositionInRow() {
		params.PositionInRow = pgtype.Int4{Int32: req.Msg.GetPositionInRow(), Valid: true}
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

	return connect.NewResponse(&emptypb.Empty{}), nil
}
