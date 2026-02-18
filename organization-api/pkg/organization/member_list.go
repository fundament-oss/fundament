package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListMembers(
	ctx context.Context,
	req *connect.Request[organizationv1.ListMembersRequest],
) (*connect.Response[organizationv1.ListMembersResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanListMembers(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	members, err := s.queries.MemberList(ctx)
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

func memberFromListRow(m *db.MemberListRow) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:         m.ID.String(),
		UserId:     m.UserID.String(),
		Name:       m.Name,
		Permission: string(m.Permission),
		Status:     string(m.Status),
		Created:    timestamppb.New(m.Created.Time),
	}

	if m.ExternalRef.Valid {
		member.ExternalRef = &m.ExternalRef.String
	}

	if m.Email.Valid {
		member.Email = &m.Email.String
	}

	return member
}
