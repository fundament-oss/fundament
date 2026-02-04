package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
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

	if err := s.checkPermission(ctx, authz.RelationMember, authz.OrganizationObject(organizationID)); err != nil {
		return nil, err
	}

	members, err := s.queries.MemberListByOrganizationID(ctx, db.MemberListByOrganizationIDParams{OrganizationID: organizationID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list members: %w", err))
	}

	result := make([]*organizationv1.Member, 0, len(members))
	for i := range members {
		result = append(result, memberFromListRow(&members[i]))
	}

	return connect.NewResponse(&organizationv1.ListMembersResponse{
		Members: result,
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

	if err := s.checkPermission(ctx, authz.RelationAdmin, authz.OrganizationObject(organizationID)); err != nil {
		return nil, err
	}

	email := req.Msg.Email
	role := req.Msg.Role

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
		Member: memberFromInviteRow(&member),
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

	if err := s.checkPermission(ctx, authz.RelationAdmin, authz.OrganizationObject(organizationID)); err != nil {
		return nil, err
	}

	memberID := uuid.MustParse(req.Msg.Id)

	err := s.queries.MemberDelete(ctx, db.MemberDeleteParams{
		ID:             memberID,
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete member: %w", err))
	}

	return connect.NewResponse(&organizationv1.DeleteMemberResponse{}), nil
}

func memberFromListRow(m *db.MemberListByOrganizationIDRow) *organizationv1.Member {
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
