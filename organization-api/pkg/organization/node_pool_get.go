package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.GetNodePoolRequest],
) (*connect.Response[organizationv1.GetNodePoolResponse], error) {
	nodePoolID := uuid.MustParse(req.Msg.NodePoolId)

	nodePool, err := s.queries.NodePoolGetByID(ctx, db.NodePoolGetByIDParams{ID: nodePoolID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get node pool: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetNodePoolResponse{
		NodePool: nodePoolFromRow(&nodePool),
	}), nil
}

func nodePoolFromRow(row *db.TenantNodePool) *organizationv1.NodePool {
	return &organizationv1.NodePool{
		Id:           row.ID.String(),
		Name:         row.Name,
		MachineType:  row.MachineType,
		CurrentNodes: 0, // Stub: would come from actual cluster state
		MinNodes:     row.AutoscaleMin,
		MaxNodes:     row.AutoscaleMax,
		Status:       organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, // Stub
		Version:      "",                                                         // Stub: would come from actual cluster state
	}
}
