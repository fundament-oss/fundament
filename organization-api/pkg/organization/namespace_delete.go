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

func (s *Server) DeleteNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteNamespaceRequest],
) (*connect.Response[organizationv1.DeleteNamespaceResponse], error) {
	namespaceID := uuid.MustParse(req.Msg.GetNamespaceId())

	namespace, err := s.queries.NamespaceGetByID(ctx, db.NamespaceGetByIDParams{ID: namespaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get namespace: %w", err))
	}

	if err := s.checkPermission(ctx, authz.CanDeleteNamespace(), authz.Project(namespace.ProjectID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.NamespaceDelete(ctx, db.NamespaceDeleteParams{ID: namespaceID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete namespace: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace not found"))
	}

	s.logger.InfoContext(ctx, "namespace deleted", "namespace_id", namespaceID)

	return connect.NewResponse(organizationv1.DeleteNamespaceResponse_builder{}.Build()), nil
}
