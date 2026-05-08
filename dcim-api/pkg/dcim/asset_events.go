package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetAssetEvents(
	ctx context.Context,
	req *connect.Request[dcimv1.GetAssetEventsRequest],
) (*connect.Response[dcimv1.GetAssetEventsResponse], error) {
	assetID := uuid.MustParse(req.Msg.GetAssetId())

	rows, err := s.queries.AssetEventList(ctx, db.AssetEventListParams{AssetID: assetID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list asset events: %w", err))
	}

	events := make([]*dcimv1.AssetEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, assetEventFromRow(&row))
	}

	return connect.NewResponse(dcimv1.GetAssetEventsResponse_builder{
		Events: events,
	}.Build()), nil
}
