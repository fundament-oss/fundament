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

// GetPlacementByAsset returns the active placement for an asset, or an unset
// placement when the asset has not been placed.
func (s *Server) GetPlacementByAsset(
	ctx context.Context,
	req *connect.Request[dcimv1.GetPlacementByAssetRequest],
) (*connect.Response[dcimv1.GetPlacementByAssetResponse], error) {
	assetID := uuid.MustParse(req.Msg.GetAssetId())

	placement, err := s.queries.PlacementGetByAsset(ctx, db.PlacementGetByAssetParams{AssetID: assetID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return connect.NewResponse(dcimv1.GetPlacementByAssetResponse_builder{}.Build()), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get placement by asset: %w", err))
	}

	return connect.NewResponse(dcimv1.GetPlacementByAssetResponse_builder{
		Placement: placementFromGetByAssetRow(&placement),
	}.Build()), nil
}
