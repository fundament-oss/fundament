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
	req *dcimv1.UpdateConnectionRequest,
) (*emptypb.Empty, error) {
	connID := uuid.MustParse(req.GetId())

	params := db.LogicalConnectionUpdateParams{
		ID: connID,
	}

	if req.HasSourcePortRole() {
		params.APortRole = pgtype.Text{String: req.GetSourcePortRole(), Valid: true}
	}

	if req.HasTargetPortRole() {
		params.BPortRole = pgtype.Text{String: req.GetTargetPortRole(), Valid: true}
	}

	if req.HasConnectionType() {
		params.ConnectionType = pgtype.Text{String: logicalConnectionTypeToDB(req.GetConnectionType()), Valid: true}
	}

	if req.HasRequirements() {
		params.Requirements = pgtype.Text{String: req.GetRequirements(), Valid: true}
	}

	if req.HasLabel() {
		params.Label = pgtype.Text{String: req.GetLabel(), Valid: true}
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

	return &emptypb.Empty{}, nil
}
