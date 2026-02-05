package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListClusterNamespaces(
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
		result = append(result, clusterNamespaceFromRow(&namespaces[i]))
	}

	return connect.NewResponse(&organizationv1.ListClusterNamespacesResponse{
		Namespaces: result,
	}), nil
}

func (s *Server) ListProjectNamespaces(
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
		result = append(result, projectNamespaceFromRow(&namespaces[i]))
	}

	return connect.NewResponse(&organizationv1.ListProjectNamespacesResponse{
		Namespaces: result,
	}), nil
}

func clusterNamespaceFromRow(row *db.TenantNamespace) *organizationv1.ClusterNamespace {
	return &organizationv1.ClusterNamespace{
		Id:        row.ID.String(),
		Name:      row.Name,
		ProjectId: row.ProjectID.String(),
		Created: timestamppb.New(row.Created.Time),
	}
}

func projectNamespaceFromRow(row *db.TenantNamespace) *organizationv1.ProjectNamespace {
	return &organizationv1.ProjectNamespace{
		Id:        row.ID.String(),
		Name:      row.Name,
		ClusterId: row.ClusterID.String(),
		Created: timestamppb.New(row.Created.Time),
	}
}
