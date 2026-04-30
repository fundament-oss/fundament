package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNamespaceRequest],
) (*connect.Response[organizationv1.CreateNamespaceResponse], error) {
	projectID := uuid.MustParse(req.Msg.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanCreateNamespace(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	params := db.NamespaceCreateParams{
		ProjectID: projectID,
		Name:      req.Msg.GetName(),
	}

	namespaceID, err := s.queries.NamespaceCreate(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.UniqueViolation {
			if pgErr.ConstraintName == dbconst.ConstraintNamespacesUqName {
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a namespace with this name already exists in the cluster"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create namespace: %w", err))
	}

	s.logger.InfoContext(ctx, "namespace created",
		"namespace_id", namespaceID,
		"project_id", projectID,
		"name", req.Msg.GetName(),
	)

	return connect.NewResponse(organizationv1.CreateNamespaceResponse_builder{
		NamespaceId: namespaceID.String(),
	}.Build()), nil
}
