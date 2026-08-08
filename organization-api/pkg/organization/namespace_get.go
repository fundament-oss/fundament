package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetNamespace(
	ctx context.Context,
	req *organizationv1.GetNamespaceRequest,
) (*organizationv1.GetNamespaceResponse, error) {
	namespaceID := uuid.MustParse(req.GetNamespaceId())

	namespace, err := s.queries.NamespaceGetByID(ctx, db.NamespaceGetByIDParams{ID: namespaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get namespace: %w", err))
	}

	// Auth is done after the DB call because we don't know the namespace ID yet.
	if err := s.checkPermission(ctx, authz.CanView(), authz.Namespace(namespace.ID)); err != nil {
		return nil, err
	}

	return organizationv1.GetNamespaceResponse_builder{
		Namespace: namespaceFromRow((db.NamespaceListByClusterIDRow)(namespace)),
	}.Build(), nil
}

func (s *Server) GetNamespaceByProjectAndName(
	ctx context.Context,
	req *organizationv1.GetNamespaceByProjectAndNameRequest,
) (*organizationv1.GetNamespaceByProjectAndNameResponse, error) {
	namespace, err := s.queries.NamespaceGetByProjectAndName(ctx, db.NamespaceGetByProjectAndNameParams{
		ClusterName:   req.GetClusterName(),
		ProjectName:   req.GetProjectName(),
		NamespaceName: req.GetNamespaceName(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get namespace: %w", err))
	}

	// Auth is done after the DB call because we don't know the namespace ID yet.
	if err := s.checkPermission(ctx, authz.CanView(), authz.Namespace(namespace.ID)); err != nil {
		return nil, err
	}

	return organizationv1.GetNamespaceByProjectAndNameResponse_builder{
		Namespace: namespaceFromRow((db.NamespaceListByClusterIDRow)(namespace)),
	}.Build(), nil
}
