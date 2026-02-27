package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListClusterNamespaces(
	ctx context.Context,
	req *connect.Request[organizationv1.ListClusterNamespacesRequest],
) (*connect.Response[organizationv1.ListClusterNamespacesResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanListNamespaces(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	namespaces, err := s.queries.NamespaceListByClusterID(ctx, db.NamespaceListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces: %w", err))
	}

	result := make([]*organizationv1.Namespace, 0, len(namespaces))
	for i := range namespaces {
		result = append(result, namespaceFromRow(namespaces[i]))
	}

	return connect.NewResponse(organizationv1.ListClusterNamespacesResponse_builder{
		Namespaces: result,
	}.Build()), nil
}

func (s *Server) ListProjectNamespaces(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectNamespacesRequest],
) (*connect.Response[organizationv1.ListProjectNamespacesResponse], error) {
	projectID := uuid.MustParse(req.Msg.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanListNamespaces(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	namespaces, err := s.queries.NamespaceListByProjectID(ctx, db.NamespaceListByProjectIDParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces: %w", err))
	}

	result := make([]*organizationv1.Namespace, 0, len(namespaces))
	for i := range namespaces {
		result = append(result, namespaceFromRow((db.NamespaceListByClusterIDRow)(namespaces[i])))
	}

	return connect.NewResponse(organizationv1.ListProjectNamespacesResponse_builder{
		Namespaces: result,
	}.Build()), nil
}

func namespaceFromRow(row db.NamespaceListByClusterIDRow) *organizationv1.Namespace {
	return organizationv1.Namespace_builder{
		Id:        row.ID.String(),
		Name:      row.Name,
		ProjectId: row.ProjectID.String(),
		ClusterId: row.ClusterID.String(),
		Created:   timestamppb.New(row.Created.Time),
	}.Build()
}
