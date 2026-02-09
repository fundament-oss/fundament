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

func (s *Server) ListNodePools(
	ctx context.Context,
	req *connect.Request[organizationv1.ListNodePoolsRequest],
) (*connect.Response[organizationv1.ListNodePoolsResponse], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	nodePools, err := s.queries.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list node pools: %w", err))
	}

	result := make([]*organizationv1.NodePool, 0, len(nodePools))
	for i := range nodePools {
		result = append(result, nodePoolFromListRow(&nodePools[i]))
	}

	return connect.NewResponse(&organizationv1.ListNodePoolsResponse{
		NodePools: result,
	}), nil
}

func nodePoolFromListRow(row *db.TenantNodePool) *organizationv1.NodePool {
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
