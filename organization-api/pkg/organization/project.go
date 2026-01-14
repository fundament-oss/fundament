package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListProjects(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectsRequest],
) (*connect.Response[organizationv1.ListProjectsResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	projects, err := s.queries.ProjectListByOrganizationID(ctx, db.ProjectListByOrganizationIDParams{OrganizationID: organizationID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list projects: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListProjectsResponse{
		Projects: adapter.FromProjects(projects),
	}), nil
}

func (s *OrganizationServer) GetProject(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectRequest],
) (*connect.Response[organizationv1.GetProjectResponse], error) {
	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	project, err := s.queries.ProjectGetByID(ctx, db.ProjectGetByIDParams{ID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetProjectResponse{
		Project: adapter.FromProject(&project),
	}), nil
}

func (s *OrganizationServer) CreateProject(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateProjectRequest],
) (*connect.Response[organizationv1.CreateProjectResponse], error) {
	input := adapter.ToProjectCreate(req.Msg)
	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	// Get user ID from context - the creator becomes the admin
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	params := db.ProjectCreateWithMemberParams{
		OrganizationID: organizationID,
		Name:           input.Name,
		UserID:         userID,
	}

	projectID, err := s.queries.ProjectCreateWithMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create project: %w", err))
	}

	s.logger.InfoContext(ctx, "project created",
		"project_id", projectID,
		"organization_id", organizationID,
		"user_id", userID,
		"name", input.Name,
	)

	return connect.NewResponse(&organizationv1.CreateProjectResponse{
		ProjectId: projectID.String(),
	}), nil
}

func (s *OrganizationServer) UpdateProject(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateProjectRequest],
) (*connect.Response[emptypb.Empty], error) {
	input, err := adapter.ToProjectUpdate(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	params := db.ProjectUpdateParams{
		ID: input.ProjectID,
	}

	if input.Name != nil {
		params.Name = pgtype.Text{String: *input.Name, Valid: true}
	}

	rowsAffected, err := s.queries.ProjectUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update project: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
	}

	s.logger.InfoContext(ctx, "project updated", "project_id", input.ProjectID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) DeleteProject(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteProjectRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	rowsAffected, err := s.queries.ProjectDelete(ctx, db.ProjectDeleteParams{ID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete project: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
	}

	s.logger.InfoContext(ctx, "project deleted", "project_id", projectID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
