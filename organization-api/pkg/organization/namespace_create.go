package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/kubename"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNamespaceRequest],
) (*connect.Response[organizationv1.CreateNamespaceResponse], error) {
	projectID := uuid.MustParse(req.Msg.GetProjectId())

	// Retry: a namespace is often created right after its project, before the
	// project's authz tuple has synced to OpenFGA (see checkPermissionWithRetry).
	if err := s.checkPermissionWithRetry(ctx, authz.CanCreateNamespace(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	// The name is materialized verbatim into a v1/Namespace on the shoot, so reject
	// anything that isn't a usable (DNS-1123, non-reserved, length-bounded) name
	// here rather than letting the cluster-worker sync fail indefinitely.
	if err := kubename.ValidateNamespace(req.Msg.GetName()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	params := db.NamespaceCreateParams{
		ProjectID: projectID,
		Name:      req.Msg.GetName(),
	}

	namespaceID, err := s.queries.NamespaceCreate(ctx, params)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == dbconst.ConstraintNamespacesUqName {
				return nil, connect.NewError(connect.CodeAlreadyExists,
					fmt.Errorf("a namespace with the name %q already exists in this project", req.Msg.GetName()))
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
