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

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateProjectMemberRole(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateProjectMemberRoleRequest],
) (*connect.Response[emptypb.Empty], error) {
	memberID := uuid.MustParse(req.Msg.GetMemberId())

	role := projectMemberRoleToDB(req.Msg.GetRole())
	if role == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role"))
	}

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.ProjectMember(memberID)); err != nil {
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

	return connect.NewResponse(&emptypb.Empty{}), nil
}
