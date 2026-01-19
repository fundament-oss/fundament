package adapter

import (
	"time"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromMembers(members []db.MemberListByOrganizationIDRow) []*organizationv1.Member {
	result := make([]*organizationv1.Member, 0, len(members))
	for i := range members {
		result = append(result, FromMemberListRow(&members[i]))
	}
	return result
}

func FromMemberListRow(m *db.MemberListByOrganizationIDRow) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:   m.ID.String(),
		Name: m.Name,
		Role: m.Role,
		CreatedAt: &organizationv1.Timestamp{
			Value: m.Created.Time.Format(time.RFC3339),
		},
	}

	if m.ExternalID.Valid {
		member.ExternalId = &m.ExternalID.String
	}

	if m.Email.Valid {
		member.Email = &m.Email.String
	}

	return member
}

func FromMemberInviteRow(m *db.MemberInviteRow) *organizationv1.Member {
	member := &organizationv1.Member{
		Id:   m.ID.String(),
		Name: m.Name,
		Role: m.Role,
		CreatedAt: &organizationv1.Timestamp{
			Value: m.Created.Time.Format(time.RFC3339),
		},
	}

	if m.ExternalID.Valid {
		member.ExternalId = &m.ExternalID.String
	}

	if m.Email.Valid {
		member.Email = &m.Email.String
	}

	return member
}
