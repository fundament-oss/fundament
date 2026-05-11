package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreateRack(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateRackRequest],
) (*connect.Response[dcimv1.CreateRackResponse], error) {
	params := db.RackCreateParams{
		RackRowID:     uuid.MustParse(req.Msg.GetRowId()),
		Name:          req.Msg.GetName(),
		TotalUnits:    req.Msg.GetTotalUnits(),
		PositionInRow: req.Msg.GetPositionInRow(),
	}

	id, err := s.queries.RackCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintRacksUqRackRowName:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("rack with this name already exists in this rack row"))
			case dbconst.ConstraintDcimRacksFkRackRow:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack row not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create rack: %w", err))
	}

	s.logger.InfoContext(ctx, "rack created", "rack_id", id)

	return connect.NewResponse(dcimv1.CreateRackResponse_builder{
		RackId: id.String(),
	}.Build()), nil
}
