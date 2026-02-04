package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListProjects(
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
		result = append(result, projectFromListRow(&projects[i]))
	}

	return connect.NewResponse(&organizationv1.ListProjectsResponse{
		Projects: result,
	}), nil
}

func projectFromListRow(row *db.TenantProject) *organizationv1.Project {
	return &organizationv1.Project{
		Id:        row.ID.String(),
		Name:      row.Name,
		CreatedAt: timestamppb.New(row.Created.Time),
	}
}
