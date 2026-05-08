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

func (s *Server) UpdateConnection(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdateConnectionRequest],
) (*connect.Response[emptypb.Empty], error) {
	connID := uuid.MustParse(req.Msg.GetId())

	params := db.LogicalConnectionUpdateParams{
		ID: connID,
	}

	if req.Msg.HasSourcePortRole() {
		params.APortRole = pgtype.Text{String: req.Msg.GetSourcePortRole(), Valid: true}
	}

	if req.Msg.HasTargetPortRole() {
		params.BPortRole = pgtype.Text{String: req.Msg.GetTargetPortRole(), Valid: true}
	}

	if req.Msg.HasConnectionType() {
		params.ConnectionType = pgtype.Text{String: logicalConnectionTypeToDB(req.Msg.GetConnectionType()), Valid: true}
	}

	if req.Msg.HasRequirements() {
		params.Requirements = pgtype.Text{String: req.Msg.GetRequirements(), Valid: true}
	}

	if req.Msg.HasLabel() {
		params.Label = pgtype.Text{String: req.Msg.GetLabel(), Valid: true}
	}

	rowsAffected, err := s.queries.LogicalConnectionUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintLogicalConnectionsCkConnectionType {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid connection type"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update connection: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("connection not found"))
	}

	s.logger.InfoContext(ctx, "connection updated", "connection_id", connID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
