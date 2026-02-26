package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteNodePoolRequest],
) (*connect.Response[organizationv1.DeleteNodePoolResponse], error) {
	nodePoolID := uuid.MustParse(req.Msg.GetNodePoolId())

	if err := s.checkPermission(ctx, authz.CanDelete(), authz.NodePool(nodePoolID)); err != nil {
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
