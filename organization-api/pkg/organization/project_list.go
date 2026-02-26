package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListProjects(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectsRequest],
) (*connect.Response[organizationv1.ListProjectsResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanListProjects(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	projects, err := s.queries.ProjectListByClusterID(ctx, db.ProjectListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list projects: %w", err))
	}

	result := make([]*organizationv1.Project, 0, len(projects))
	for i := range projects {
		result = append(result, projectFromListRow(&projects[i]))
	}

	return connect.NewResponse(organizationv1.ListProjectsResponse_builder{
		Projects: result,
	}.Build()), nil
}

func projectFromListRow(row *db.TenantProject) *organizationv1.Project {
	return organizationv1.Project_builder{
		Id:        row.ID.String(),
		ClusterId: row.ClusterID.String(),
		Name:      row.Name,
		Created:   timestamppb.New(row.Created.Time),
	}.Build()
}
