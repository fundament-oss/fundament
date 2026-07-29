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
	req *dcimv1.ListConnectionsByPlacementRequest,
) (*dcimv1.ListConnectionsByPlacementResponse, error) {
	placementID := uuid.MustParse(req.GetPlacementId())

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

	return dcimv1.ListConnectionsByPlacementResponse_builder{
		Connections: connections,
	}.Build(), nil
}

func (s *Server) ListConnectionsBySite(
	ctx context.Context,
	req *dcimv1.ListConnectionsBySiteRequest,
) (*dcimv1.ListConnectionsBySiteResponse, error) {
	siteID := uuid.MustParse(req.GetSiteId())

	rows, err := s.queries.PhysicalConnectionListBySite(ctx, db.PhysicalConnectionListBySiteParams{
		SiteID: siteID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list physical connections: %w", err))
	}

	connections := make([]*dcimv1.PhysicalConnection, 0, len(rows))
	for _, row := range rows {
		connections = append(connections, physicalConnectionFromListBySiteRow(&row))
	}

	return dcimv1.ListConnectionsBySiteResponse_builder{
		Connections: connections,
	}.Build(), nil
}
