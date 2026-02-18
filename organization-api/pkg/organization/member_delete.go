package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteMember(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteMemberRequest],
) (*connect.Response[organizationv1.DeleteMemberResponse], error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	id := uuid.MustParse(req.Msg.Id)

	memberUserID, err := s.queries.MemberGetUserID(ctx, db.MemberGetUserIDParams{ID: id})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
	}

	if memberUserID == userID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot remove yourself"))
	}

	err = s.queries.MemberDelete(ctx, db.MemberDeleteParams{
		ID: id,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete member: %w", err))
	}

	return connect.NewResponse(&organizationv1.DeleteMemberResponse{}), nil
}
