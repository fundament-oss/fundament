package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateProject(
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
