package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetNamespaceByClusterAndName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetNamespaceByClusterAndNameRequest],
) (*connect.Response[organizationv1.GetNamespaceByClusterAndNameResponse], error) {
	namespace, err := s.queries.NamespaceGetByClusterAndName(ctx, db.NamespaceGetByClusterAndNameParams{
		ClusterName:   req.Msg.ClusterName,
		NamespaceName: req.Msg.NamespaceName,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace not found"))
		}
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
	namespace, err := s.queries.NamespaceGetByProjectAndName(ctx, db.NamespaceGetByProjectAndNameParams{
		ProjectName:   req.Msg.ProjectName,
		NamespaceName: req.Msg.NamespaceName,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get namespace: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetNamespaceByProjectAndNameResponse{
		Namespace: clusterNamespaceFromRow(&namespace),
	}), nil
}
