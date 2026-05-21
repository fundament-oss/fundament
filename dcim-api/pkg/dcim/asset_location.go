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
// placement up to the rack-bearing host, then joining the rack hierarchy
// (rack -> rack_row -> room -> site) to human-readable names.
func (s *Server) GetAssetLocation(
	ctx context.Context,
	req *connect.Request[dcimv1.GetAssetLocationRequest],
) (*connect.Response[dcimv1.GetAssetLocationResponse], error) {
	assetID := uuid.MustParse(req.Msg.GetAssetId())

	rackRef, err := s.queries.PlacementResolveRackByAsset(ctx, db.PlacementResolveRackByAssetParams{
		AssetID: assetID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Asset has no rack-based placement; return an unset location.
			return connect.NewResponse(dcimv1.GetAssetLocationResponse_builder{}.Build()), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve asset rack: %w", err))
	}

	rack, err := s.queries.RackGetByID(ctx, db.RackGetByIDParams{ID: uuid.UUID(rackRef.RackID.Bytes)})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rack: %w", err))
	}

	rackRow, err := s.queries.RackRowGetByID(ctx, db.RackRowGetByIDParams{ID: rack.RackRowID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rack row: %w", err))
	}

	room, err := s.queries.RoomGetByID(ctx, db.RoomGetByIDParams{ID: rackRow.RoomID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get room: %w", err))
	}

	site, err := s.queries.SiteGetByID(ctx, db.SiteGetByIDParams{ID: room.SiteID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get site: %w", err))
	}

	return connect.NewResponse(dcimv1.GetAssetLocationResponse_builder{
		Location: dcimv1.AssetLocation_builder{
			SiteName:      site.Name,
			RoomName:      room.Name,
			RackRowName:   rackRow.Name,
			RackName:      rack.Name,
			RackUnitStart: rackRef.StartUnit.Int32,
			RackId:        uuid.UUID(rackRef.RackID.Bytes).String(),
			RackSlotType:  rackSlotTypeToProto(rackRef.SlotType.String),
		}.Build(),
	}.Build()), nil
}
