package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListNodePools(
	ctx context.Context,
	req *organizationv1.ListNodePoolsRequest,
) (*organizationv1.ListNodePoolsResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanListNodePools(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

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

	runtimes := s.nodePoolRuntimes(ctx, clusterID, len(nodePools))

	result := make([]*organizationv1.NodePool, 0, len(nodePools))
	for i := range nodePools {
		result = append(result, nodePoolFromRow(&nodePools[i], runtimes[nodePools[i].Name]))
	}

	return organizationv1.ListNodePoolsResponse_builder{
		NodePools: result,
	}.Build(), nil
}
