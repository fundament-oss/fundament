package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetRackRow(
	ctx context.Context,
	req *connect.Request[dcimv1.GetRackRowRequest],
) (*connect.Response[dcimv1.GetRackRowResponse], error) {
	rackRowID := uuid.MustParse(req.Msg.GetId())

	rackRow, err := s.queries.RackRowGetByID(ctx, db.RackRowGetByIDParams{ID: rackRowID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack row not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rack row: %w", err))
	}

	return connect.NewResponse(dcimv1.GetRackRowResponse_builder{
		RackRow: rackRowFromRow(&rackRow),
	}.Build()), nil
}
