package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) InviteMember(
	ctx context.Context,
	req *connect.Request[organizationv1.InviteMemberRequest],
) (*connect.Response[organizationv1.InviteMemberResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	email := req.Msg.Email
	role := req.Msg.Role

	params := db.MemberGetByEmailParams{
		Email: email,
	}

	// Check if email is already a member of this organization
	_, err := s.queries.MemberGetByEmail(ctx, params)
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("email is already in use"))
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check email: %w", err))
	}

	// Create the user record
	userRow, err := s.queries.MemberInviteUser(ctx, db.MemberInviteUserParams{
		Email: email,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create invited user: %w", err))
	}

	// Create the organization membership
	membershipRow, err := s.queries.MemberInviteMembership(ctx, db.MemberInviteMembershipParams{
		OrganizationID: organizationID,
		UserID:         userRow.ID,
		Role:           role,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create membership: %w", err))
	}

	return connect.NewResponse(&organizationv1.InviteMemberResponse{
		Member: memberFromInviteRows(&userRow, &membershipRow),
	}), nil
}

func memberFromInviteRows(u *db.MemberInviteUserRow, m *db.MemberInviteMembershipRow) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:      u.ID.String(),
		Name:    u.Name,
		Role:    m.Role,
		Created: timestamppb.New(m.Created.Time),
	}

	if u.ExternalRef.Valid {
		member.ExternalRef = &u.ExternalRef.String
	}

	if u.Email.Valid {
		member.Email = &u.Email.String
	}

	return member
}
