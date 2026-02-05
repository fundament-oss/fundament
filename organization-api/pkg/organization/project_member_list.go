package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListProjectMembers(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectMembersRequest],
) (*connect.Response[organizationv1.ListProjectMembersResponse], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)

	members, err := s.queries.ProjectMemberList(ctx, db.ProjectMemberListParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list project members: %w", err))
	}

	result := make([]*organizationv1.ProjectMember, 0, len(members))
	for i := range members {
		result = append(result, projectMemberFromListRow(&members[i]))
	}

	return connect.NewResponse(&organizationv1.ListProjectMembersResponse{
		Members: result,
	}), nil
}

func projectMemberFromListRow(row *db.ProjectMemberListRow) *organizationv1.ProjectMember {
	return &organizationv1.ProjectMember{
		Id:        row.ID.String(),
		ProjectId: row.ProjectID.String(),
		UserId:    row.UserID.String(),
		UserName:  row.UserName,
		Role:      projectMemberRoleFromDB(row.Role),
		Created:   timestamppb.New(row.Created.Time),
	}
}
