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

func (s *Server) ListChildPlacements(
	ctx context.Context,
	req *dcimv1.ListChildPlacementsRequest,
) (*dcimv1.ListChildPlacementsResponse, error) {
	parentID := uuid.MustParse(req.GetParentPlacementId())

	rows, err := s.queries.PlacementListByParent(ctx, db.PlacementListByParentParams{
		ParentPlacementID: pgtype.UUID{Bytes: parentID, Valid: true},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list child placements: %w", err))
	}

	placements := make([]*dcimv1.Placement, 0, len(rows))
	for _, row := range rows {
		placements = append(placements, placementFromParentListRow(&row))
	}

	return dcimv1.ListChildPlacementsResponse_builder{
		Placements: placements,
	}.Build(), nil
}
