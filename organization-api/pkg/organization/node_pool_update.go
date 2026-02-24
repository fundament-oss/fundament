package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateNodePoolRequest],
) (*connect.Response[emptypb.Empty], error) {
	nodePoolID := uuid.MustParse(req.Msg.NodePoolId)

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.NodePool(nodePoolID)); err != nil {
		return nil, err
	}

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
