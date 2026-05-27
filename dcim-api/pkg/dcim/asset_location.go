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

// GetAssetLocation resolves an asset's physical location by walking its
// placement up to the rack-bearing host and joining the rack hierarchy
// (rack -> rack_row -> room -> site) to human-readable names.
func (s *Server) GetAssetLocation(
	ctx context.Context,
	req *connect.Request[dcimv1.GetAssetLocationRequest],
) (*connect.Response[dcimv1.GetAssetLocationResponse], error) {
	assetID := uuid.MustParse(req.Msg.GetAssetId())

	loc, err := s.queries.PlacementResolveLocationByAsset(ctx, db.PlacementResolveLocationByAssetParams{
		AssetID: assetID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Asset has no rack-based placement; return an unset location.
			return connect.NewResponse(dcimv1.GetAssetLocationResponse_builder{}.Build()), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve asset location: %w", err))
	}

	return connect.NewResponse(dcimv1.GetAssetLocationResponse_builder{
		Location: dcimv1.AssetLocation_builder{
			SiteName:      loc.SiteName,
			RoomName:      loc.RoomName,
			RackRowName:   loc.RackRowName,
			RackName:      loc.RackName,
			RackUnitStart: loc.StartUnit.Int32,
			RackId:        uuid.UUID(loc.RackID.Bytes).String(),
			RackSlotType:  rackSlotTypeToProto(loc.SlotType.String),
		}.Build(),
	}.Build()), nil
}
