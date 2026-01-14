package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListProjectMembers(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectMembersRequest],
) (*connect.Response[organizationv1.ListProjectMembersResponse], error) {
	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	members, err := s.queries.ProjectMemberList(ctx, db.ProjectMemberListParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list project members: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListProjectMembersResponse{
		Members: adapter.FromProjectMembers(members),
	}), nil
}

func (s *OrganizationServer) AddProjectMember(
	ctx context.Context,
	req *connect.Request[organizationv1.AddProjectMemberRequest],
) (*connect.Response[organizationv1.AddProjectMemberResponse], error) {
	projectID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project id: %w", err))
	}

	userID, err := uuid.Parse(req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid user id: %w", err))
	}

	role := adapter.ToProjectMemberRole(req.Msg.Role)
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	memberID, err := s.queries.ProjectMemberCreate(ctx, db.ProjectMemberCreateParams{
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to add project member: %w", err))
	}

	s.logger.InfoContext(ctx, "project member added",
		"member_id", memberID,
		"project_id", projectID,
		"user_id", userID,
		"role", role,
	)

	return connect.NewResponse(&organizationv1.AddProjectMemberResponse{
		MemberId: memberID.String(),
	}), nil
}

func (s *OrganizationServer) UpdateProjectMemberRole(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateProjectMemberRoleRequest],
) (*connect.Response[emptypb.Empty], error) {
	memberID, err := uuid.Parse(req.Msg.MemberId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid member id: %w", err))
	}

	role := adapter.ToProjectMemberRole(req.Msg.Role)
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	// Get the member to find project_id and check current role
	member, err := s.queries.ProjectMemberGetByID(ctx, db.ProjectMemberGetByIDParams{ID: memberID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// If demoting from admin, check this isn't the last admin
	if role == "member" && member.Role == "admin" {
		adminCount, err := s.queries.ProjectMemberCountAdmins(ctx, db.ProjectMemberCountAdminsParams{ProjectID: member.ProjectID})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if adminCount <= 1 {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("cannot demote the last admin"))
		}
	}

	rowsAffected, err := s.queries.ProjectMemberUpdateRole(ctx, db.ProjectMemberUpdateRoleParams{
		ID:   memberID,
		Role: role,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update member role: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	s.logger.InfoContext(ctx, "project member role updated",
		"member_id", memberID,
		"role", role,
	)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) RemoveProjectMember(
	ctx context.Context,
	req *connect.Request[organizationv1.RemoveProjectMemberRequest],
) (*connect.Response[emptypb.Empty], error) {
	memberID, err := uuid.Parse(req.Msg.MemberId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid member id: %w", err))
	}

	// Check if this is the last admin before removing
	member, err := s.queries.ProjectMemberGetByID(ctx, db.ProjectMemberGetByIDParams{ID: memberID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if member.Role == "admin" {
		adminCount, err := s.queries.ProjectMemberCountAdmins(ctx, db.ProjectMemberCountAdminsParams{ProjectID: member.ProjectID})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if adminCount <= 1 {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("cannot remove the last admin"))
		}
	}

	rowsAffected, err := s.queries.ProjectMemberDelete(ctx, db.ProjectMemberDeleteParams{ID: memberID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to remove member: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	s.logger.InfoContext(ctx, "project member removed",
		"member_id", memberID,
	)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
