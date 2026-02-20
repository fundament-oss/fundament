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

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) RemoveProjectMember(
	ctx context.Context,
	req *connect.Request[organizationv1.RemoveProjectMemberRequest],
) (*connect.Response[emptypb.Empty], error) {
	memberID := uuid.MustParse(req.Msg.MemberId)

	rowsAffected, err := s.queries.ProjectMemberDelete(ctx, db.ProjectMemberDeleteParams{ID: memberID})
	if err != nil {

		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
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
