package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListAssetsByCatalogEntry(
	ctx context.Context,
	req *connect.Request[dcimv1.ListAssetsByCatalogEntryRequest],
) (*connect.Response[dcimv1.ListAssetsByCatalogEntryResponse], error) {
	catalogID := uuid.MustParse(req.Msg.GetDeviceCatalogId())

	rows, err := s.queries.AssetListByCatalogID(ctx, db.AssetListByCatalogIDParams{DeviceCatalogID: catalogID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list assets by catalog entry: %w", err))
	}

	assets := make([]*dcimv1.Asset, 0, len(rows))
	for _, row := range rows {
		assets = append(assets, assetFromListByCatalogRow(&row))
	}

	return connect.NewResponse(dcimv1.ListAssetsByCatalogEntryResponse_builder{
		Assets: assets,
	}.Build()), nil
}
