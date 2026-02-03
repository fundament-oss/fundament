package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ListClusterNamespaces lists all namespaces for a cluster
func (s *OrganizationServer) ListClusterNamespaces(
	ctx context.Context,
	req *connect.Request[organizationv1.ListClusterNamespacesRequest],
) (*connect.Response[organizationv1.ListClusterNamespacesResponse], error) {
	if _, ok := OrganizationIDFromContext(ctx); !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	clusterID := uuid.MustParse(req.Msg.ClusterId)

	namespaces, err := s.queries.NamespaceListByClusterID(ctx, db.NamespaceListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces: %w", err))
	}

	result := make([]*organizationv1.ClusterNamespace, 0, len(namespaces))
	for i := range namespaces {
		result = append(result, &organizationv1.ClusterNamespace{
			Id:        namespaces[i].ID.String(),
			Name:      namespaces[i].Name,
			ProjectId: namespaces[i].ProjectID.String(),
			CreatedAt: timestamppb.New(namespaces[i].Created.Time),
		})
	}

	return connect.NewResponse(&organizationv1.ListClusterNamespacesResponse{
		Namespaces: result,
	}), nil
}

// CreateNamespace creates a new namespace in a cluster
func (s *OrganizationServer) CreateNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNamespaceRequest],
) (*connect.Response[organizationv1.CreateNamespaceResponse], error) {
	if _, ok := OrganizationIDFromContext(ctx); !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	projectID := uuid.MustParse(req.Msg.ProjectId)
	clusterID := uuid.MustParse(req.Msg.ClusterId)

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

// DeleteNamespace deletes a namespace
func (s *OrganizationServer) DeleteNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteNamespaceRequest],
) (*connect.Response[emptypb.Empty], error) {
	if _, ok := OrganizationIDFromContext(ctx); !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	namespaceID := uuid.MustParse(req.Msg.NamespaceId)

	rowsAffected, err := s.queries.NamespaceDelete(ctx, db.NamespaceDeleteParams{ID: namespaceID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete namespace: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace not found"))
	}

	s.logger.InfoContext(ctx, "namespace deleted", "namespace_id", namespaceID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListProjectNamespaces lists all namespaces belonging to a project
func (s *OrganizationServer) ListProjectNamespaces(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectNamespacesRequest],
) (*connect.Response[organizationv1.ListProjectNamespacesResponse], error) {
	if _, ok := OrganizationIDFromContext(ctx); !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	projectID := uuid.MustParse(req.Msg.ProjectId)

	namespaces, err := s.queries.NamespaceListByProjectID(ctx, db.NamespaceListByProjectIDParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces: %w", err))
	}

	result := make([]*organizationv1.ProjectNamespace, 0, len(namespaces))
	for i := range namespaces {
		result = append(result, &organizationv1.ProjectNamespace{
			Id:        namespaces[i].ID.String(),
			Name:      namespaces[i].Name,
			ClusterId: namespaces[i].ClusterID.String(),
			CreatedAt: timestamppb.New(namespaces[i].Created.Time),
		})
	}

	return connect.NewResponse(&organizationv1.ListProjectNamespacesResponse{
		Namespaces: result,
	}), nil
}
