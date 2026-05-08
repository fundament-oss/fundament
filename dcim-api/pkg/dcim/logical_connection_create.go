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

func (s *Server) CreateConnection(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateConnectionRequest],
) (*connect.Response[dcimv1.CreateConnectionResponse], error) {
	params := db.LogicalConnectionCreateParams{
		LogicalDesignID:  uuid.MustParse(req.Msg.GetDesignId()),
		ALogicalDeviceID: uuid.MustParse(req.Msg.GetSourceDeviceId()),
		APortRole:        pgtype.Text{String: req.Msg.GetSourcePortRole(), Valid: true},
		BLogicalDeviceID: uuid.MustParse(req.Msg.GetTargetDeviceId()),
		BPortRole:        pgtype.Text{String: req.Msg.GetTargetPortRole(), Valid: true},
		ConnectionType:   logicalConnectionTypeToDB(req.Msg.GetConnectionType()),
	}

	if req.Msg.HasRequirements() {
		params.Requirements = pgtype.Text{String: req.Msg.GetRequirements(), Valid: true}
	}

	if req.Msg.GetLabel() != "" {
		params.Label = pgtype.Text{String: req.Msg.GetLabel(), Valid: true}
	}

	id, err := s.queries.LogicalConnectionCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimLogicalConnectionsFkDesign:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("design not found"))
			case dbconst.ConstraintDcimLogicalConnectionsFkADevice:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("source device not found"))
			case dbconst.ConstraintDcimLogicalConnectionsFkBDevice:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("target device not found"))
			case dbconst.ConstraintLogicalConnectionsCkConnectionType:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid connection type"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create connection: %w", err))
	}

	s.logger.InfoContext(ctx, "connection created", "connection_id", id)

	return connect.NewResponse(dcimv1.CreateConnectionResponse_builder{
		ConnectionId: id.String(),
	}.Build()), nil
}
