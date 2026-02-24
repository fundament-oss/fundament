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
	req *connect.Request[organizationv1.GetNamespaceRequest],
) (*connect.Response[organizationv1.GetNamespaceResponse], error) {
	namespaceID := uuid.MustParse(req.Msg.NamespaceId)

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

	return connect.NewResponse(&organizationv1.GetNamespaceResponse{
		Namespace: namespaceFromRow((db.NamespaceListByClusterIDRow)(namespace)),
	}), nil
}

func (s *Server) GetNamespaceByProjectAndName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetNamespaceByProjectAndNameRequest],
) (*connect.Response[organizationv1.GetNamespaceByProjectAndNameResponse], error) {
	namespace, err := s.queries.NamespaceGetByProjectAndName(ctx, db.NamespaceGetByProjectAndNameParams{
		ClusterName:   req.Msg.ClusterName,
		ProjectName:   req.Msg.ProjectName,
		NamespaceName: req.Msg.NamespaceName,
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

	return connect.NewResponse(&organizationv1.GetNamespaceByProjectAndNameResponse{
		Namespace: namespaceFromRow((db.NamespaceListByClusterIDRow)(namespace)),
	}), nil
}
