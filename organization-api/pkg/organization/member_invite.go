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

	_, err := s.queries.MemberGetByEmail(ctx, db.MemberGetByEmailParams{
		Email:          email,
		OrganizationID: organizationID,
	})
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("email is already in use"))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check email: %w", err))
	}

	member, err := s.queries.MemberInvite(ctx, db.MemberInviteParams{
		OrganizationID: organizationID,
		Name:           email,
		Role:           role,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to invite member: %w", err))
	}

	return connect.NewResponse(&organizationv1.InviteMemberResponse{
		Member: memberFromInviteRow(&member),
	}), nil
}

func memberFromInviteRow(m *db.MemberInviteRow) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:        m.ID.String(),
		Name:      m.Name,
		Role:      m.Role,
		CreatedAt: timestamppb.New(m.Created.Time),
	}

	if m.ExternalID.Valid {
		member.ExternalId = &m.ExternalID.String
	}

	if m.Email.Valid {
		member.Email = &m.Email.String
	}

	return member
}

