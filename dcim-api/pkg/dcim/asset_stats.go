package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetAssetStats(
	ctx context.Context,
	req *connect.Request[dcimv1.GetAssetStatsRequest],
) (*connect.Response[dcimv1.GetAssetStatsResponse], error) {
	row, err := s.queries.AssetStats(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get asset stats: %w", err))
	}

	return connect.NewResponse(dcimv1.GetAssetStatsResponse_builder{
		Stats: dcimv1.AssetStats_builder{
			Total:          row.Total,
			Available:      row.Available,
			Deployed:       row.Deployed,
			NeedsRepair:    row.NeedsRepair,
			OnOrder:        row.OnOrder,
			Requested:      row.Requested,
			Decommissioned: row.Decommissioned,
		}.Build(),
	}.Build()), nil
}
