package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteClusterRequest],
) (*connect.Response[emptypb.Empty], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	if err := s.checkPermission(ctx, authz.CanDelete(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.ClusterDelete(ctx, db.ClusterDeleteParams{ID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete cluster: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	s.logger.InfoContext(ctx, "cluster deleted", "cluster_id", clusterID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
