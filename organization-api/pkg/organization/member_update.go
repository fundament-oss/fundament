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

func (s *Server) UpdateMemberPermission(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateMemberPermissionRequest],
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

	member, err := s.queries.MemberGetByID(ctx, db.MemberGetByIDParams{ID: memberID})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	if member.UserID == userID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot modify your own permission"))
	}

	rowsAffected, err := s.queries.MemberUpdatePermission(ctx, db.MemberUpdatePermissionParams{
		ID:             memberID,
		Permission:     dbconst.OrganizationsUserPermission(req.Msg.Permission),
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update member permission: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	s.logger.InfoContext(ctx, "organization member permission updated",
		"member_id", memberID,
		"permission", req.Msg.Permission,
	)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
