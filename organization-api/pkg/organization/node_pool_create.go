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

func (s *Server) CreateNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNodePoolRequest],
) (*connect.Response[organizationv1.CreateNodePoolResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanCreateNodePool(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	params := db.NodePoolCreateParams{
		ClusterID:    clusterID,
		Name:         req.Msg.GetName(),
		MachineType:  req.Msg.GetMachineType(),
		AutoscaleMin: req.Msg.GetAutoscaleMin(),
		AutoscaleMax: req.Msg.GetAutoscaleMax(),
	}

	nodePoolID, err := s.queries.NodePoolCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create node pool: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool created",
		"node_pool_id", nodePoolID,
		"cluster_id", clusterID,
		"name", req.Msg.GetName(),
	)

	return connect.NewResponse(organizationv1.CreateNodePoolResponse_builder{
		NodePoolId: nodePoolID.String(),
	}.Build()), nil
}
