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

func (s *Server) GetAsset(
	ctx context.Context,
	req *dcimv1.GetAssetRequest,
) (*dcimv1.GetAssetResponse, error) {
	assetID := uuid.MustParse(req.GetId())

	row, err := s.queries.AssetGetByID(ctx, db.AssetGetByIDParams{ID: assetID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("asset not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get asset: %w", err))
	}

	return dcimv1.GetAssetResponse_builder{
		Asset: assetFromGetRow(&row),
	}.Build(), nil
}
