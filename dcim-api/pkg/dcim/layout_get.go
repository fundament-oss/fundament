package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetLayout(
	ctx context.Context,
	req *dcimv1.GetLayoutRequest,
) (*dcimv1.GetLayoutResponse, error) {
	designID := uuid.MustParse(req.GetDesignId())

	rows, err := s.queries.LogicalDeviceLayoutGetByDesign(ctx, db.LogicalDeviceLayoutGetByDesignParams{LogicalDesignID: designID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get layout: %w", err))
	}

	positions := make([]*dcimv1.LogicalDeviceLayout, 0, len(rows))
	for _, row := range rows {
		positions = append(positions, layoutFromRow(&row))
	}

	return dcimv1.GetLayoutResponse_builder{
		Positions: positions,
	}.Build(), nil
}
