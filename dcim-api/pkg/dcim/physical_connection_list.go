package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListConnectionsByPlacement(
	ctx context.Context,
	req *connect.Request[dcimv1.ListConnectionsByPlacementRequest],
) (*connect.Response[dcimv1.ListConnectionsByPlacementResponse], error) {
	placementID := uuid.MustParse(req.Msg.GetPlacementId())

	rows, err := s.queries.PhysicalConnectionListByPlacement(ctx, db.PhysicalConnectionListByPlacementParams{
		APlacementID: placementID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list physical connections: %w", err))
	}

	connections := make([]*dcimv1.PhysicalConnection, 0, len(rows))
	for _, row := range rows {
		connections = append(connections, physicalConnectionFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListConnectionsByPlacementResponse_builder{
		Connections: connections,
	}.Build()), nil
}
