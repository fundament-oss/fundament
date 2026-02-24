package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) AddProjectMember(
	ctx context.Context,
	req *connect.Request[organizationv1.AddProjectMemberRequest],
) (*connect.Response[organizationv1.AddProjectMemberResponse], error) {
	projectID := uuid.MustParse(req.Msg.ProjectId)
	userID := uuid.MustParse(req.Msg.UserId)

	role := projectMemberRoleToDB(req.Msg.Role)
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	if err := s.checkPermission(ctx, authz.CanManageMembers(), authz.Project(projectID)); err != nil {
		return nil, err
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

	return connect.NewResponse(&organizationv1.AddProjectMemberResponse{
		MemberId: memberID.String(),
	}), nil
}
