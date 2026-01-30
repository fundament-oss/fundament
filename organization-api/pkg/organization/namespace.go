package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
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

	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	namespaces, err := s.queries.NamespaceListByClusterID(ctx, db.NamespaceListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListClusterNamespacesResponse{
		Namespaces: adapter.FromClusterNamespaces(namespaces),
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

	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	input := models.NamespaceCreate{
		Name: req.Msg.Name,
	}

	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	params := db.NamespaceCreateParams{
		ProjectID: projectID,
		ClusterID: clusterID,
		Name:      input.Name,
	}

	namespaceID, err := s.queries.NamespaceCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create namespace: %w", err))
	}

	s.logger.InfoContext(ctx, "namespace created",
		"namespace_id", namespaceID,
		"project_id", projectID,
		"cluster_id", clusterID,
		"name", input.Name,
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

	namespaceID, err := uuid.Parse(req.Msg.NamespaceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid namespace id: %w", err))
	}

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

	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	namespaces, err := s.queries.NamespaceListByProjectID(ctx, db.NamespaceListByProjectIDParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListProjectNamespacesResponse{
		Namespaces: adapter.FromProjectNamespaces(namespaces),
	}), nil
}

// GetNamespaceByClusterAndName gets a namespace by cluster name and namespace name
func (s *OrganizationServer) GetNamespaceByClusterAndName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetNamespaceByClusterAndNameRequest],
) (*connect.Response[organizationv1.GetNamespaceByClusterAndNameResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	namespace, err := s.queries.NamespaceGetByClusterAndName(ctx, db.NamespaceGetByClusterAndNameParams{
		OrganizationID: organizationID,
		ClusterName:    req.Msg.ClusterName,
		NamespaceName:  req.Msg.NamespaceName,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get namespace: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetNamespaceByClusterAndNameResponse{
		Namespace: adapter.FromClusterNamespace(&namespace),
	}), nil
}

// GetNamespaceByProjectAndName gets a namespace by project name and namespace name
func (s *OrganizationServer) GetNamespaceByProjectAndName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetNamespaceByProjectAndNameRequest],
) (*connect.Response[organizationv1.GetNamespaceByProjectAndNameResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	namespace, err := s.queries.NamespaceGetByProjectAndName(ctx, db.NamespaceGetByProjectAndNameParams{
		OrganizationID: organizationID,
		ProjectName:    req.Msg.ProjectName,
		NamespaceName:  req.Msg.NamespaceName,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get namespace: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetNamespaceByProjectAndNameResponse{
		Namespace: adapter.FromClusterNamespace(&namespace),
	}), nil
}
