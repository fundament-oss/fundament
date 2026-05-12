package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListConnections(
	ctx context.Context,
	req *connect.Request[dcimv1.ListConnectionsRequest],
) (*connect.Response[dcimv1.ListConnectionsResponse], error) {
	designID := uuid.MustParse(req.Msg.GetDesignId())

	rows, err := s.queries.LogicalConnectionList(ctx, db.LogicalConnectionListParams{LogicalDesignID: designID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list connections: %w", err))
	}

	connections := make([]*dcimv1.LogicalConnection, 0, len(rows))
	for _, row := range rows {
		connections = append(connections, logicalConnectionFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListConnectionsResponse_builder{
		Connections: connections,
	}.Build()), nil
}
