package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) AcceptInvitation(
	ctx context.Context,
	req *organizationv1.AcceptInvitationRequest,
) (*organizationv1.AcceptInvitationResponse, error) {
	id := uuid.MustParse(req.GetId())

	rows, err := s.queries.InviteAccept(ctx, db.InviteAcceptParams{ID: id})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to accept invitation: %w", err))
	}

	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no pending invitation found"))
	}

	return organizationv1.AcceptInvitationResponse_builder{}.Build(), nil
}
