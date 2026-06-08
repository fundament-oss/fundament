package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListPlacementsByRack(
	ctx context.Context,
	req *connect.Request[dcimv1.ListPlacementsByRackRequest],
) (*connect.Response[dcimv1.ListPlacementsByRackResponse], error) {
	rackID := uuid.MustParse(req.Msg.GetRackId())

	rows, err := s.queries.PlacementListByRack(ctx, db.PlacementListByRackParams{
		RackID: pgtype.UUID{Bytes: rackID, Valid: true},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list placements by rack: %w", err))
	}

	placements := make([]*dcimv1.Placement, 0, len(rows))
	for _, row := range rows {
		placements = append(placements, placementFromRackListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListPlacementsByRackResponse_builder{
		Placements: placements,
	}.Build()), nil
}
