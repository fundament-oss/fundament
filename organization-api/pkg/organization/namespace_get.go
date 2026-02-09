package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetNamespaceByClusterAndName(
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
		Namespace: clusterNamespaceFromRow(&namespace),
	}), nil
}

func (s *Server) GetNamespaceByProjectAndName(
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
		Namespace: clusterNamespaceFromRow(&namespace),
	}), nil
}
