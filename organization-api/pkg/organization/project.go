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

	params := db.ProjectCreateParams{
		OrganizationID: organizationID,
		Name:           input.Name,
	}

	projectID, err := s.queries.ProjectCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create project: %w", err))
	}

	s.logger.InfoContext(ctx, "project created",
		"project_id", projectID,
		"organization_id", organizationID,
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

func (s *OrganizationServer) AttachNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.AttachNamespaceRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	namespaceID, err := uuid.Parse(req.Msg.NamespaceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid namespace id: %w", err))
	}

	params := db.NamespaceProjectAttachParams{
		NamespaceID: namespaceID,
		ProjectID:   projectID,
	}

	if err := s.queries.NamespaceProjectAttach(ctx, params); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to attach namespace to project: %w", err))
	}

	s.logger.InfoContext(ctx, "namespace attached to project",
		"project_id", projectID,
		"namespace_id", namespaceID,
	)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) DetachNamespace(
	ctx context.Context,
	req *connect.Request[organizationv1.DetachNamespaceRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	namespaceID, err := uuid.Parse(req.Msg.NamespaceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid namespace id: %w", err))
	}

	params := db.NamespaceProjectDetachParams{
		NamespaceID: namespaceID,
		ProjectID:   projectID,
	}

	rowsAffected, err := s.queries.NamespaceProjectDetach(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to detach namespace from project: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("namespace-project attachment not found"))
	}

	s.logger.InfoContext(ctx, "namespace detached from project",
		"project_id", projectID,
		"namespace_id", namespaceID,
	)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) ListNamespaces(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectNamespacesRequest],
) (*connect.Response[organizationv1.ListProjectNamespacesResponse], error) {
	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	namespaces, err := s.queries.NamespaceProjectListByProjectID(ctx, db.NamespaceProjectListByProjectIDParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list namespaces for project: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListProjectNamespacesResponse{
		Namespaces: adapter.FromProjectNamespaces(namespaces),
	}), nil
}
