package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateMemberRole(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateMemberRoleRequest],
) (*connect.Response[emptypb.Empty], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	memberID := uuid.MustParse(req.Msg.Id)

	if memberID == userID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot modify your own role"))
	}

	rowsAffected, err := s.queries.MemberUpdateRole(ctx, db.MemberUpdateRoleParams{
		ID:             memberID,
		Permission:     dbconst.OrganizationsUserPermission(req.Msg.Role),
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update member role: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	s.logger.InfoContext(ctx, "organization member role updated",
		"member_id", memberID,
		"role", req.Msg.Role,
	)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
