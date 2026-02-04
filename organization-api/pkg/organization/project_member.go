package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListProjectMembers(
	ctx context.Context,
	req *connect.Request[organizationv1.ListProjectMembersRequest],
) (*connect.Response[organizationv1.ListProjectMembersResponse], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)

	if err := s.checkPermission(ctx, authz.RelationCanView, authz.ProjectObject(projectID)); err != nil {
		return nil, err
	}

	members, err := s.queries.ProjectMemberList(ctx, db.ProjectMemberListParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list project members: %w", err))
	}

	result := make([]*organizationv1.ProjectMember, 0, len(members))
	for i := range members {
		result = append(result, &organizationv1.ProjectMember{
			Id:        members[i].ID.String(),
			ProjectId: members[i].ProjectID.String(),
			UserId:    members[i].UserID.String(),
			UserName:  members[i].UserName,
			Role:      projectMemberRoleFromDB(members[i].Role),
			CreatedAt: timestamppb.New(members[i].Created.Time),
		})
	}

	return connect.NewResponse(&organizationv1.ListProjectMembersResponse{
		Members: result,
	}), nil
}

func (s *OrganizationServer) AddProjectMember(
	ctx context.Context,
	req *connect.Request[organizationv1.AddProjectMemberRequest],
) (*connect.Response[organizationv1.AddProjectMemberResponse], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)
	userID := uuid.MustParse(req.Msg.UserId)

	role := projectMemberRoleToDB(req.Msg.Role)
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	if err := s.checkPermission(ctx, authz.RelationCanManageMembers, authz.ProjectObject(projectID)); err != nil {
		return nil, err
	}

	memberID, err := s.queries.ProjectMemberCreate(ctx, db.ProjectMemberCreateParams{
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == dbconst.ConstraintProjectMembersUqProjectUser {
				return nil, connect.NewError(connect.CodeAlreadyExists,
					fmt.Errorf("user is already a member of this project"))
			}
		}

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
	memberID := uuid.MustParse(req.Msg.MemberId)

	role := projectMemberRoleToDB(req.Msg.Role)
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	// Look up member to get project_id and current role for authz check and tuple management
	member, err := s.queries.ProjectMemberGetByID(ctx, db.ProjectMemberGetByIDParams{ID: memberID})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	if err := s.checkPermission(ctx, authz.RelationCanManageMembers, authz.ProjectObject(member.ProjectID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.ProjectMemberUpdateRole(ctx, db.ProjectMemberUpdateRoleParams{
		ID:   memberID,
		Role: role,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintProjectContainsOneAdmin {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot demote the last admin"))
			}
		}

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
	memberID := uuid.MustParse(req.Msg.MemberId)

	// Look up member to get project_id and role for authz check and tuple deletion
	member, err := s.queries.ProjectMemberGetByID(ctx, db.ProjectMemberGetByIDParams{ID: memberID})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	if err := s.checkPermission(ctx, authz.RelationCanManageMembers, authz.ProjectObject(member.ProjectID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.ProjectMemberDelete(ctx, db.ProjectMemberDeleteParams{ID: memberID})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.RaiseException &&
				pgErr.Hint == dbconst.HintProjectContainsOneAdmin {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot remove the last admin"))
			}
		}

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

func projectMemberRoleFromDB(role dbconst.ProjectMemberRole) organizationv1.ProjectMemberRole {
	switch role {
	case dbconst.ProjectMemberRole_Admin:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN
	case dbconst.ProjectMemberRole_Viewer:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER
	default:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED
	}
}

func projectMemberRoleToDB(role organizationv1.ProjectMemberRole) dbconst.ProjectMemberRole {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return dbconst.ProjectMemberRole_Admin
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return dbconst.ProjectMemberRole_Viewer
	default:
		return ""
	}
}
