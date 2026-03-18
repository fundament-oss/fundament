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

func (s *Server) UpdateProjectMemberRole(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateProjectMemberRoleRequest],
) (*connect.Response[organizationv1.UpdateProjectMemberRoleResponse], error) {
	memberID := uuid.MustParse(req.Msg.GetMemberId())

	role := projectMemberRoleToDB(req.Msg.GetRole())
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	member, err := s.queries.ProjectMemberGetByID(ctx, db.ProjectMemberGetByIDParams{ID: memberID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project member: %w", err))
	}

	if err := s.checkPermission(ctx, authz.CanEditProjectMember(), authz.Project(member.ProjectID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.ProjectMemberUpdateRole(ctx, db.ProjectMemberUpdateRoleParams{
		ID:   memberID,
		Role: role,
	})
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
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

	return connect.NewResponse(organizationv1.UpdateProjectMemberRoleResponse_builder{}.Build()), nil
}
