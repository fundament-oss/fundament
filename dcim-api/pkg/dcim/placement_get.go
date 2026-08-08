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

func (s *Server) GetPlacement(
	ctx context.Context,
	req *dcimv1.GetPlacementRequest,
) (*dcimv1.GetPlacementResponse, error) {
	placementID := uuid.MustParse(req.GetId())

	placement, err := s.queries.PlacementGetByID(ctx, db.PlacementGetByIDParams{ID: placementID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("placement not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get placement: %w", err))
	}

	return dcimv1.GetPlacementResponse_builder{
		Placement: placementFromGetRow(&placement),
	}.Build(), nil
}
