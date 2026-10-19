package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) AddProjectMember(
	ctx context.Context,
	req *organizationv1.AddProjectMemberRequest,
) (*organizationv1.AddProjectMemberResponse, error) {
	projectID := uuid.MustParse(req.GetProjectId())
	userID := uuid.MustParse(req.GetUserId())

	role := projectMemberRoleToDB(req.GetRole())
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	if err := s.checkPermission(ctx, authz.CanManageMembers(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	// Only accepted organization members can join a project: a membership for
	// a pending invitee or a removed member grants nothing on the clusters, and
	// would come back into effect when they join the organization later.
	orgMember, err := s.queries.MemberGetByUserID(ctx, db.MemberGetByUserIDParams{UserID: userID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get organization member: %w", err))
	}
	if err != nil || orgMember.Status != dbconst.OrganizationsUserStatus_Accepted {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("user is not an accepted member of the organization"))
	}

	memberID, err := s.queries.ProjectMemberCreate(ctx, db.ProjectMemberCreateParams{
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
	})
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
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

	return organizationv1.AddProjectMemberResponse_builder{
		MemberId: memberID.String(),
	}.Build(), nil
}
