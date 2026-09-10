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

func (s *Server) GetNodePool(
	ctx context.Context,
	req *organizationv1.GetNodePoolRequest,
) (*organizationv1.GetNodePoolResponse, error) {
	nodePoolID := uuid.MustParse(req.GetNodePoolId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.NodePool(nodePoolID)); err != nil {
		return nil, err
	}

	nodePool, err := s.queries.NodePoolGetByID(ctx, db.NodePoolGetByIDParams{ID: nodePoolID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get node pool: %w", err))
	}

	runtime := s.nodePoolRuntimes(ctx, nodePool.ClusterID, 1)[nodePool.Name]

	return organizationv1.GetNodePoolResponse_builder{
		NodePool: nodePoolFromRow(&nodePool, runtime),
	}.Build(), nil
}

// The pool as the database holds it, filled out with what the cluster reports
// about it. An empty runtime (no metrics backend) leaves the live fields at
// their zero value, which the console reads as "unknown".
func nodePoolFromRow(row *db.TenantNodePool, runtime nodePoolRuntime) *organizationv1.NodePool {
	return organizationv1.NodePool_builder{
		Id:           row.ID.String(),
		Name:         row.Name,
		MachineType:  row.MachineType,
		CurrentNodes: runtime.nodes,
		MinNodes:     row.AutoscaleMin,
		MaxNodes:     row.AutoscaleMax,
		Status:       runtime.status(),
		Version:      runtime.version,
	}.Build()
}
