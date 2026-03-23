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

func (s *Server) DeleteNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteNodePoolRequest],
) (*connect.Response[organizationv1.DeleteNodePoolResponse], error) {
	nodePoolID := uuid.MustParse(req.Msg.GetNodePoolId())

	nodePool, err := s.queries.NodePoolGetByID(ctx, db.NodePoolGetByIDParams{ID: nodePoolID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get node pool: %w", err))
	}

	if err := s.checkPermission(ctx, authz.CanDeleteNodePool(), authz.Cluster(nodePool.ClusterID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.NodePoolDelete(ctx, db.NodePoolDeleteParams{ID: nodePoolID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete node pool: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
	}

	s.logger.InfoContext(ctx, "node pool deleted", "node_pool_id", nodePoolID)

	return connect.NewResponse(organizationv1.DeleteNodePoolResponse_builder{}.Build()), nil
}
