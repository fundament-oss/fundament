package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetProjectByName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectByNameRequest],
) (*connect.Response[organizationv1.GetProjectResponse], error) {
	project, err := s.queries.ProjectGetByName(ctx, db.ProjectGetByNameParams{
		Name: req.Msg.Name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// Auth is done after the DB call because we don't know the project ID yet.
	if err := s.checkPermission(ctx, authz.CanView(), authz.Project(project.ID)); err != nil {
		return nil, err
	}

	return connect.NewResponse(&organizationv1.GetProjectResponse{
		Project: projectFromGetRow(&project),
	}), nil
}

func (s *Server) GetProject(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectRequest],
) (*connect.Response[organizationv1.GetProjectResponse], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)

	if err := s.checkPermission(ctx, authz.CanView(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	project, err := s.queries.ProjectGetByID(ctx, db.ProjectGetByIDParams{ID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetProjectResponse{
		Project: projectFromGetRow(&project),
	}), nil
}

func projectFromGetRow(row *db.TenantProject) *organizationv1.Project {
	return &organizationv1.Project{
		Id:        row.ID.String(),
		ClusterId: row.ClusterID.String(),
		Name:      row.Name,
		Created:   timestamppb.New(row.Created.Time),
	}
}
