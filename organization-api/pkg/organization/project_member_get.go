package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetProjectMember(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectMemberRequest],
) (*connect.Response[organizationv1.GetProjectMemberResponse], error) {
	memberID := uuid.MustParse(req.Msg.MemberId)

	if err := s.checkPermission(ctx, authz.CanView(), authz.ProjectMember(memberID)); err != nil {
		return nil, err
	}

	member, err := s.queries.ProjectMemberGetByID(ctx, db.ProjectMemberGetByIDParams{ID: memberID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project member: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetProjectMemberResponse{
		Member: projectMemberFromGetRow(&member),
	}), nil
}

func projectMemberFromGetRow(row *db.ProjectMemberGetByIDRow) *organizationv1.ProjectMember {
	return &organizationv1.ProjectMember{
		Id:        row.ID.String(),
		ProjectId: row.ProjectID.String(),
		UserId:    row.UserID.String(),
		UserName:  row.UserName,
		Role:      projectMemberRoleFromDB(row.Role),
		Created:   timestamppb.New(row.Created.Time),
	}
}
