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

func (s *Server) CreateNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNamespaceRequest],
) (*connect.Response[organizationv1.CreateNamespaceResponse], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	if err := s.checkPermission(ctx, authz.CanCreateNamespace(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	params := db.NamespaceCreateParams{
		ProjectID: projectID,
		ClusterID: clusterID,
		Name:      req.Msg.Name,
	}

	namespaceID, err := s.queries.NamespaceCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create namespace: %w", err))
	}

	s.logger.InfoContext(ctx, "namespace created",
		"namespace_id", namespaceID,
		"project_id", projectID,
		"cluster_id", clusterID,
		"name", req.Msg.Name,
	)

	return connect.NewResponse(&organizationv1.CreateNamespaceResponse{
		NamespaceId: namespaceID.String(),
	}), nil
}
