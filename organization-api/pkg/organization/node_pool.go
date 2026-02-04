package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) CreateNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNodePoolRequest],
) (*connect.Response[organizationv1.CreateNodePoolResponse], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	// Verify cluster exists
	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	params := db.NodePoolCreateParams{
		ClusterID:    clusterID,
		Name:         req.Msg.Name,
		MachineType:  req.Msg.MachineType,
		AutoscaleMin: req.Msg.AutoscaleMin,
		AutoscaleMax: req.Msg.AutoscaleMax,
	}

	nodePoolID, err := s.queries.NodePoolCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create node pool: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool created",
		"node_pool_id", nodePoolID,
		"cluster_id", clusterID,
		"name", req.Msg.Name,
	)

	return connect.NewResponse(&organizationv1.CreateNodePoolResponse{
		NodePoolId: nodePoolID.String(),
	}), nil
}

func (s *OrganizationServer) UpdateNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateNodePoolRequest],
) (*connect.Response[emptypb.Empty], error) {
	nodePoolID := uuid.MustParse(req.Msg.NodePoolId)

	params := db.NodePoolUpdateParams{
		ID:           nodePoolID,
		AutoscaleMin: pgtype.Int4{Int32: req.Msg.AutoscaleMin, Valid: true},
		AutoscaleMax: pgtype.Int4{Int32: req.Msg.AutoscaleMax, Valid: true},
	}

	rowsAffected, err := s.queries.NodePoolUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update node pool: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
	}

	s.logger.InfoContext(ctx, "node pool updated", "node_pool_id", nodePoolID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) DeleteNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteNodePoolRequest],
) (*connect.Response[emptypb.Empty], error) {
	nodePoolID := uuid.MustParse(req.Msg.NodePoolId)

	rowsAffected, err := s.queries.NodePoolDelete(ctx, db.NodePoolDeleteParams{ID: nodePoolID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete node pool: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
	}

	s.logger.InfoContext(ctx, "node pool deleted", "node_pool_id", nodePoolID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) ListNodePools(
	ctx context.Context,
	req *connect.Request[organizationv1.ListNodePoolsRequest],
) (*connect.Response[organizationv1.ListNodePoolsResponse], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	// Verify cluster exists
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
	for _, np := range nodePools {
		result = append(result, &organizationv1.NodePool{
			Id:           np.ID.String(),
			Name:         np.Name,
			MachineType:  np.MachineType,
			CurrentNodes: 0, // Stub: would come from actual cluster state
			MinNodes:     np.AutoscaleMin,
			MaxNodes:     np.AutoscaleMax,
			Status:       organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, // Stub
			Version:      "",                                                         // Stub: would come from actual cluster state
		})
	}

	return connect.NewResponse(&organizationv1.ListNodePoolsResponse{
		NodePools: result,
	}), nil
}

func (s *OrganizationServer) GetNodePool(
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
		NodePool: &organizationv1.NodePool{
			Id:           nodePool.ID.String(),
			Name:         nodePool.Name,
			MachineType:  nodePool.MachineType,
			CurrentNodes: 0, // Stub: would come from actual cluster state
			MinNodes:     nodePool.AutoscaleMin,
			MaxNodes:     nodePool.AutoscaleMax,
			Status:       organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, // Stub
			Version:      "",                                                         // Stub: would come from actual cluster state
		},
	}), nil
}
