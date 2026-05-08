package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) SaveLayout(
	ctx context.Context,
	req *connect.Request[dcimv1.SaveLayoutRequest],
) (*connect.Response[dcimv1.SaveLayoutResponse], error) {
	designID := uuid.MustParse(req.Msg.GetDesignId())

	positions := make([]*dcimv1.LogicalDeviceLayout, 0, len(req.Msg.GetPositions()))
	for _, pos := range req.Msg.GetPositions() {
		row, err := s.queries.LogicalDeviceLayoutUpsert(ctx, db.LogicalDeviceLayoutUpsertParams{
			LogicalDeviceID: uuid.MustParse(pos.GetDeviceId()),
			PositionX:       float64ToNumeric(pos.GetPositionX()),
			PositionY:       float64ToNumeric(pos.GetPositionY()),
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save layout: %w", err))
		}

		positions = append(positions, layoutFromUpsertRow(&row))
	}

	s.logger.InfoContext(ctx, "layout saved", "design_id", designID)

	return connect.NewResponse(dcimv1.SaveLayoutResponse_builder{
		Positions: positions,
	}.Build()), nil
}
