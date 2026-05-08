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

func (s *Server) UpdateDesign(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdateDesignRequest],
) (*connect.Response[emptypb.Empty], error) {
	designID := uuid.MustParse(req.Msg.GetId())

	params := db.LogicalDesignUpdateParams{
		ID: designID,
	}

	if req.Msg.HasName() {
		params.Name = pgtype.Text{String: req.Msg.GetName(), Valid: true}
	}

	if req.Msg.HasDescription() {
		params.Description = pgtype.Text{String: req.Msg.GetDescription(), Valid: true}
	}

	if req.Msg.HasStatus() {
		params.Status = pgtype.Text{String: logicalDesignStatusToDB(req.Msg.GetStatus()), Valid: true}
	}

	rowsAffected, err := s.queries.LogicalDesignUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintLogicalDesignsUqName:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("design with this name already exists"))
			case dbconst.ConstraintLogicalDesignsCkStatus:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid design status"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update design: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("design not found"))
	}

	s.logger.InfoContext(ctx, "design updated", "design_id", designID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
