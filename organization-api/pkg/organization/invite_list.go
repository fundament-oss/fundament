package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListInvitations(
	ctx context.Context,
	req *connect.Request[organizationv1.ListInvitationsRequest],
) (*connect.Response[organizationv1.ListInvitationsResponse], error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	rows, err := s.queries.InviteList(ctx, db.InviteListParams{
		UserID: userID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list invitations: %w", err))
	}

	invitations := make([]*organizationv1.Invitation, 0, len(rows))
	for i := range rows {
		invitations = append(invitations, &organizationv1.Invitation{
			Id:                      rows[i].ID.String(),
			OrganizationId:          rows[i].OrganizationID.String(),
			OrganizationDisplayName: rows[i].DisplayName,
			Permission:              string(rows[i].Permission),
			Created:                 timestamppb.New(rows[i].Created.Time),
		})
	}

	return connect.NewResponse(&organizationv1.ListInvitationsResponse{
		Invitations: invitations,
	}), nil
}
