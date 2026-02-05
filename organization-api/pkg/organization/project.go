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
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
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

	result := make([]*organizationv1.Project, 0, len(projects))
	for i := range projects {
		result = append(result, &organizationv1.Project{
			Id:        projects[i].ID.String(),
			Name:      projects[i].Name,
			CreatedAt: timestamppb.New(projects[i].Created.Time),
		})
	}

	return connect.NewResponse(&organizationv1.ListProjectsResponse{
		Projects: result,
	}), nil
}

func (s *OrganizationServer) GetProject(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectRequest],
) (*connect.Response[organizationv1.GetProjectResponse], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)

	project, err := s.queries.ProjectGetByID(ctx, db.ProjectGetByIDParams{ID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetProjectResponse{
		Project: &organizationv1.Project{
			Id:        project.ID.String(),
			Name:      project.Name,
			CreatedAt: timestamppb.New(project.Created.Time),
		},
	}), nil
}

func (s *OrganizationServer) GetProjectByName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectByNameRequest],
) (*connect.Response[organizationv1.GetProjectResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	project, err := s.queries.ProjectGetByName(ctx, db.ProjectGetByNameParams{
		OrganizationID: organizationID,
		Name:           req.Msg.Name,
	})
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
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	// Get user ID from context - the creator becomes the admin
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	s.logger.DebugContext(ctx, "creating project with member",
		"organization_id", organizationID,
		"user_id", userID,
		"name", req.Msg.Name,
	)

	// Start a transaction to create project and add creator as admin
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	// Create the project
	projectID, err := qtx.ProjectCreate(ctx, db.ProjectCreateParams{
		OrganizationID: organizationID,
		Name:           req.Msg.Name,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create project: %w", err))
	}

	params := db.ProjectMemberCreateParams{
		ProjectID: projectID,
		UserID:    userID,
		Role:      dbconst.ProjectMemberRole_Admin,
	}

	// Add creator as admin
	_, err = qtx.ProjectMemberCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to add project creator as admin: %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	s.logger.DebugContext(ctx, "project created",
		"project_id", projectID,
		"organization_id", organizationID,
		"user_id", userID,
		"name", req.Msg.Name,
	)

	return connect.NewResponse(&organizationv1.CreateProjectResponse{
		ProjectId: projectID.String(),
	}), nil
}

func (s *OrganizationServer) UpdateProject(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateProjectRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)

	params := db.ProjectUpdateParams{
		ID: projectID,
	}

	if req.Msg.Name != nil {
		params.Name = pgtype.Text{String: *req.Msg.Name, Valid: true}
	}

	rowsAffected, err := s.queries.ProjectUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update project: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
	}

	s.logger.InfoContext(ctx, "project updated", "project_id", projectID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) DeleteProject(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteProjectRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)

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
