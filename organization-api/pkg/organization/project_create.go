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
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateProject(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateProjectRequest],
) (*connect.Response[organizationv1.CreateProjectResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())
	if err := s.checkPermissionWithRetry(ctx, authz.CanCreateProject(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	s.logger.DebugContext(ctx, "creating project with member",
		"cluster_id", clusterID,
		"user_id", userID,
		"name", req.Msg.GetName(),
	)

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	alias := req.Msg.GetName()
	if req.Msg.HasAlias() {
		alias = req.Msg.GetAlias()
	}

	projectID, err := qtx.ProjectCreate(ctx, db.ProjectCreateParams{
		ClusterID: clusterID,
		Name:      req.Msg.GetName(),
		Alias:     alias,
	})
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.CheckViolation {
			if pgErr.ConstraintName == dbconst.ConstraintProjectsCkAlias {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("alias must be between 1 and 255 characters"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create project: %w", err))
	}

	_, err = qtx.ProjectMemberCreate(ctx, db.ProjectMemberCreateParams{
		ProjectID: projectID,
		UserID:    userID,
		Role:      dbconst.ProjectMemberRole_Admin,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to add project creator as admin: %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	s.logger.DebugContext(ctx, "project created",
		"project_id", projectID,
		"cluster_id", clusterID,
		"user_id", userID,
		"name", req.Msg.GetName(),
	)

	return connect.NewResponse(organizationv1.CreateProjectResponse_builder{
		ProjectId: projectID.String(),
	}.Build()), nil
}
