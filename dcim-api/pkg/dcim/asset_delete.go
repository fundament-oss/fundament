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

func (s *Server) DeleteAsset(
	ctx context.Context,
	req *dcimv1.DeleteAssetRequest,
) (*emptypb.Empty, error) {
	assetID := uuid.MustParse(req.GetId())

	rowsAffected, err := s.queries.AssetDelete(ctx, db.AssetDeleteParams{ID: assetID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete asset: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("asset not found"))
	}

	s.logger.InfoContext(ctx, "asset deleted", "asset_id", assetID)

	return &emptypb.Empty{}, nil
}
