package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListClusterNamespaces(
	ctx context.Context,
	req *connect.Request[organizationv1.ListClusterNamespacesRequest],
) (*connect.Response[organizationv1.ListClusterNamespacesResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	// Verify cluster exists
	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	namespaces, err := s.queries.NamespaceListByClusterID(ctx, db.NamespaceListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListClusterNamespacesResponse{
		Namespaces: adapter.FromClusterNamespaces(namespaces),
	}), nil
}
