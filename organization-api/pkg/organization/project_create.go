package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

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
	clusterID := uuid.MustParse(req.Msg.ClusterId)
	if err := s.checkPermission(ctx, authz.CanCreateProject(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	s.logger.DebugContext(ctx, "creating project with member",
		"cluster_id", clusterID,
		"user_id", userID,
		"name", req.Msg.Name,
	)

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	projectID, err := qtx.ProjectCreate(ctx, db.ProjectCreateParams{
		ClusterID: clusterID,
		Name:      req.Msg.Name,
	})
	if err != nil {
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
		"name", req.Msg.Name,
	)

	return connect.NewResponse(&organizationv1.CreateProjectResponse{
		ProjectId: projectID.String(),
	}), nil
}
