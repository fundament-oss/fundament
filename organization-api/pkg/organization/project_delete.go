package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteProject(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteProjectRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)

	if err := s.checkPermission(ctx, authz.CanDelete(), authz.Project(projectID)); err != nil {
		return nil, err
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
