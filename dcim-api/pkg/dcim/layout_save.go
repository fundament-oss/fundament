package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) SaveLayout(
	ctx context.Context,
	req *connect.Request[dcimv1.SaveLayoutRequest],
) (*connect.Response[dcimv1.SaveLayoutResponse], error) {
	designID := uuid.MustParse(req.Msg.GetDesignId())

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	devices, err := qtx.LogicalDeviceList(ctx, db.LogicalDeviceListParams{LogicalDesignID: designID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list devices for design: %w", err))
	}

	allowed := make(map[uuid.UUID]struct{}, len(devices))
	for _, d := range devices {
		allowed[d.ID] = struct{}{}
	}

	for _, pos := range req.Msg.GetPositions() {
		deviceID := uuid.MustParse(pos.GetDeviceId())
		if _, ok := allowed[deviceID]; !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("device %s does not belong to design %s", deviceID, designID))
		}
	}

	keep := make([]uuid.UUID, 0, len(req.Msg.GetPositions()))
	positions := make([]*dcimv1.LogicalDeviceLayout, 0, len(req.Msg.GetPositions()))
	for _, pos := range req.Msg.GetPositions() {
		deviceID := uuid.MustParse(pos.GetDeviceId())
		row, err := qtx.LogicalDeviceLayoutUpsert(ctx, db.LogicalDeviceLayoutUpsertParams{
			LogicalDeviceID: deviceID,
			PositionX:       float64ToNumeric(pos.GetPositionX()),
			PositionY:       float64ToNumeric(pos.GetPositionY()),
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save layout: %w", err))
		}

		keep = append(keep, deviceID)
		positions = append(positions, layoutFromUpsertRow(&row))
	}

	if err := qtx.LogicalDeviceLayoutDeleteNotIn(ctx, db.LogicalDeviceLayoutDeleteNotInParams{
		LogicalDesignID: designID,
		Keep:            keep,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to prune layout: %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	s.logger.InfoContext(ctx, "layout saved", "design_id", designID)

	return connect.NewResponse(dcimv1.SaveLayoutResponse_builder{
		Positions: positions,
	}.Build()), nil
}
