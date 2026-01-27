package organization

import (
	"context"
	"errors"
	"fmt"
	"net/mail"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListMembers(
	ctx context.Context,
	req *connect.Request[organizationv1.ListMembersRequest],
) (*connect.Response[organizationv1.ListMembersResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	members, err := s.queries.MemberListByOrganizationID(ctx, db.MemberListByOrganizationIDParams{OrganizationID: organizationID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list members: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListMembersResponse{
		Members: adapter.FromMembers(members),
	}), nil
}

func (s *OrganizationServer) InviteMember(
	ctx context.Context,
	req *connect.Request[organizationv1.InviteMemberRequest],
) (*connect.Response[organizationv1.InviteMemberResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	email := req.Msg.Email
	if email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("email is required"))
	}

	// Validate email format
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid email address"))
	}

	// Validate and default role
	role := req.Msg.Role
	if role == "" {
		role = "viewer"
	}
	if role != "viewer" && role != "admin" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("role must be 'viewer' or 'admin'"))
	}

	// Check if email is already in use within this organization
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
		Member: adapter.FromMemberInviteRow(&member),
	}), nil
}

func (s *OrganizationServer) DeleteMember(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteMemberRequest],
) (*connect.Response[organizationv1.DeleteMemberResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	memberID, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid member id: %w", err))
	}

	err = s.queries.MemberDelete(ctx, db.MemberDeleteParams{
		ID:             memberID,
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete member: %w", err))
	}

	return connect.NewResponse(&organizationv1.DeleteMemberResponse{}), nil
}
