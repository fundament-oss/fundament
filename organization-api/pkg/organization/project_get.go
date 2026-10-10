package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetProjectByName(
	ctx context.Context,
	req *organizationv1.GetProjectByNameRequest,
) (*organizationv1.GetProjectResponse, error) {
	project, err := s.queries.ProjectGetByName(ctx, db.ProjectGetByNameParams{
		Name: req.GetName(),
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

	return organizationv1.GetProjectResponse_builder{
		Project: projectMessage(project.ID, project.ClusterID, project.Name, project.Alias, project.Created),
	}.Build(), nil
}

func (s *Server) GetProject(
	ctx context.Context,
	req *organizationv1.GetProjectRequest,
) (*organizationv1.GetProjectResponse, error) {
	projectID := uuid.MustParse(req.GetProjectId())

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

	return organizationv1.GetProjectResponse_builder{
		Project: projectMessage(project.ID, project.ClusterID, project.Name, project.Alias, project.Created),
	}.Build(), nil
}

// projectMessage takes the columns rather than a row type: ProjectGetByID and
// ProjectGetByName select the same ones but sqlc gives each its own struct.
func projectMessage(id, clusterID uuid.UUID, name, alias string, created pgtype.Timestamptz) *organizationv1.Project {
	return organizationv1.Project_builder{
		Id:        id.String(),
		ClusterId: clusterID.String(),
		Name:      name,
		Alias:     alias,
		Created:   timestamppb.New(created.Time),
	}.Build()
}
