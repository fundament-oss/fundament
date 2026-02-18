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
	id := uuid.MustParse(req.Msg.Id)

	err := s.queries.MemberDelete(ctx, db.MemberDeleteParams{
		ID: id,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete member: %w", err))
	}

	return connect.NewResponse(&organizationv1.DeleteMemberResponse{}), nil
}
