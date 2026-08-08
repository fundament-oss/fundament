package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) DeletePlacement(
	ctx context.Context,
	req *dcimv1.DeletePlacementRequest,
) (*emptypb.Empty, error) {
	placementID := uuid.MustParse(req.GetId())

	rowsAffected, err := s.queries.PlacementDelete(ctx, db.PlacementDeleteParams{ID: placementID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete placement: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("placement not found"))
	}

	s.logger.InfoContext(ctx, "placement deleted", "placement_id", placementID)

	return &emptypb.Empty{}, nil
}
