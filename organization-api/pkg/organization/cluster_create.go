package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateClusterRequest],
) (*connect.Response[organizationv1.CreateClusterResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	params := db.ClusterCreateParams{
		OrganizationID:    organizationID,
		Name:              req.Msg.Name,
		Region:            req.Msg.Region,
		KubernetesVersion: req.Msg.KubernetesVersion,
	}

	clusterID, err := s.queries.ClusterCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create cluster: %w", err))
	}

	s.logger.InfoContext(ctx, "cluster created",
		"cluster_id", clusterID,
		"organization_id", organizationID,
		"name", req.Msg.Name,
		"region", req.Msg.Region,
	)

	return connect.NewResponse(&organizationv1.CreateClusterResponse{
		ClusterId: clusterID.String(),
	}), nil
}
