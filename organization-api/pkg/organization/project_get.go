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
	var clusterID pgtype.UUID
	if req.HasClusterId() {
		clusterID = pgtype.UUID{Bytes: uuid.MustParse(req.GetClusterId()), Valid: true}
	}

	projects, err := s.queries.ProjectListByName(ctx, db.ProjectListByNameParams{
		Name:      req.GetName(),
		ClusterID: clusterID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// Auth is done after the DB call because we don't know the project IDs
	// yet. Only projects the caller may view count, so the answer does not
	// reveal projects on clusters the caller cannot see.
	var visible []db.TenantProject
	var permErr error
	for i := range projects {
		if err := s.checkPermission(ctx, authz.CanView(), authz.Project(projects[i].ID)); err != nil {
			permErr = err
			continue
		}
		visible = append(visible, projects[i])
	}

	switch {
	case len(visible) == 1:
		return organizationv1.GetProjectResponse_builder{
			Project: projectFromGetRow(&visible[0]),
		}.Build(), nil
	case len(visible) > 1:
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("several clusters have a project named %q; set cluster_id", req.GetName()))
	case permErr != nil:
		return nil, permErr
	default:
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
	}
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
		Project: projectFromGetRow(&project),
	}.Build(), nil
}

func projectFromGetRow(row *db.TenantProject) *organizationv1.Project {
	return organizationv1.Project_builder{
		Id:        row.ID.String(),
		ClusterId: row.ClusterID.String(),
		Name:      row.Name,
		Alias:     row.Alias,
		Created:   timestamppb.New(row.Created.Time),
	}.Build()
}
