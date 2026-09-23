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
	"github.com/fundament-oss/fundament/common/kubename"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateNamespace(
	ctx context.Context,
	req *organizationv1.CreateNamespaceRequest,
) (*organizationv1.CreateNamespaceResponse, error) {
	projectID := uuid.MustParse(req.GetProjectId())

	// Retry: a namespace is often created right after its project, before the
	// project's authz tuple has synced to OpenFGA (see checkPermissionWithRetry).
	if err := s.checkPermissionWithRetry(ctx, authz.CanCreateNamespace(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	project, err := s.queries.ProjectGetByID(ctx, db.ProjectGetByIDParams{ID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// The name is materialized as "tnt-<project>--<name>" into a v1/Namespace on the
	// shoot, so reject anything that wouldn't be a usable (DNS-1123, non-reserved,
	// length-bounded) name here rather than letting the cluster-worker sync fail
	// indefinitely.
	err = kubename.ValidateNamespace(project.Name, req.GetName())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	clusterSideName := kubename.GenerateNamespace(project.Name, req.GetName())
	taken, err := s.queries.NamespaceClusterNameTaken(ctx, db.NamespaceClusterNameTakenParams{
		ClusterID:       project.ClusterID,
		ProjectID:       projectID,
		Prefix:          kubename.TenantNamespacePrefix,
		Separator:       kubename.NamespaceSeparator,
		ClusterSideName: clusterSideName,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check namespace name: %w", err))
	}
	if taken {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("namespace %q would be named %q on the cluster, which another project's namespace already uses", req.GetName(), clusterSideName))
	}

	params := db.NamespaceCreateParams{
		ProjectID: projectID,
		Name:      req.GetName(),
	}

	namespaceID, err := s.queries.NamespaceCreate(ctx, params)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == dbconst.ConstraintNamespacesUqName {
				return nil, connect.NewError(connect.CodeAlreadyExists,
					fmt.Errorf("a namespace with the name %q already exists in this project", req.GetName()))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create namespace: %w", err))
	}

	s.logger.InfoContext(ctx, "namespace created",
		"namespace_id", namespaceID,
		"project_id", projectID,
		"name", req.GetName(),
	)

	return organizationv1.CreateNamespaceResponse_builder{
		NamespaceId: namespaceID.String(),
	}.Build(), nil
}
