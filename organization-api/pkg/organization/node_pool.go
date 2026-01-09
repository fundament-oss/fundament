package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) CreateNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNodePoolRequest],
) (*connect.Response[organizationv1.CreateNodePoolResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	input := models.NodePoolCreate{
		Name:         req.Msg.Name,
		MachineType:  req.Msg.MachineType,
		AutoscaleMin: req.Msg.AutoscaleMin,
		AutoscaleMax: req.Msg.AutoscaleMax,
	}

	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Verify cluster exists
	if _, err := s.queries.ClusterGetByID(ctx, clusterID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	params := db.NodePoolCreateParams{
		ClusterID:    clusterID,
		Name:         input.Name,
		MachineType:  input.MachineType,
		AutoscaleMin: input.AutoscaleMin,
		AutoscaleMax: input.AutoscaleMax,
	}

	nodePool, err := s.queries.NodePoolCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create node pool: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool created",
		"node_pool_id", nodePool.ID,
		"cluster_id", clusterID,
		"name", nodePool.Name,
	)

	return connect.NewResponse(&organizationv1.CreateNodePoolResponse{
		NodePool: adapter.FromNodePool(nodePool),
	}), nil
}

func (s *OrganizationServer) UpdateNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateNodePoolRequest],
) (*connect.Response[organizationv1.UpdateNodePoolResponse], error) {
	nodePoolID, err := uuid.Parse(req.Msg.NodePoolId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid node pool id: %w", err))
	}

	input := adapter.ToNodePoolUpdate(req.Msg)

	params := db.NodePoolUpdateParams{
		ID:           nodePoolID,
		AutoscaleMin: pgtype.Int4{Int32: input.AutoscaleMin, Valid: true},
		AutoscaleMax: pgtype.Int4{Int32: input.AutoscaleMax, Valid: true},
	}

	nodePool, err := s.queries.NodePoolUpdate(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update node pool: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool updated", "node_pool_id", nodePool.ID, "name", nodePool.Name)

	return connect.NewResponse(&organizationv1.UpdateNodePoolResponse{
		NodePool: adapter.FromNodePool(nodePool),
	}), nil
}

func (s *OrganizationServer) DeleteNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteNodePoolRequest],
) (*connect.Response[organizationv1.DeleteNodePoolResponse], error) {
	nodePoolID, err := uuid.Parse(req.Msg.NodePoolId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid node pool id: %w", err))
	}

	if err := s.queries.NodePoolDelete(ctx, nodePoolID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete node pool: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool deleted", "node_pool_id", nodePoolID)

	return connect.NewResponse(&organizationv1.DeleteNodePoolResponse{
		Success: true,
	}), nil
}
