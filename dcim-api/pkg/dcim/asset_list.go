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

func (s *Server) ListAssets(
	ctx context.Context,
	req *connect.Request[dcimv1.ListAssetsRequest],
) (*connect.Response[dcimv1.ListAssetsResponse], error) {
	params := db.AssetListParams{}

	if req.Msg.GetIncludeDeleted() {
		params.IncludeDeleted = pgtype.Bool{Bool: true, Valid: true}
	}

	if req.Msg.HasStatusFilter() {
		params.Status = pgtype.Text{String: assetStatusToDB(req.Msg.GetStatusFilter()), Valid: true}
	}

	if req.Msg.HasDeviceCatalogId() {
		catalogID := uuid.MustParse(req.Msg.GetDeviceCatalogId())
		params.DeviceCatalogID = pgtype.UUID{Bytes: catalogID, Valid: true}
	}

	if req.Msg.GetSearch() != "" {
		params.Search = pgtype.Text{String: req.Msg.GetSearch(), Valid: true}
	}

	rows, err := s.queries.AssetList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list assets: %w", err))
	}

	assets := make([]*dcimv1.Asset, 0, len(rows))
	for _, row := range rows {
		assets = append(assets, assetFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListAssetsResponse_builder{
		Assets: assets,
	}.Build()), nil
}
