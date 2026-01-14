package adapter

import (
	"time"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromProjectMembers(members []db.ProjectMemberListRow) []*organizationv1.ProjectMember {
	result := make([]*organizationv1.ProjectMember, 0, len(members))
	for i := range members {
		result = append(result, FromProjectMember(&members[i]))
	}
	return result
}

func FromProjectMember(m *db.ProjectMemberListRow) *organizationv1.ProjectMember {
	return &organizationv1.ProjectMember{
		Id:        m.ID.String(),
		ProjectId: m.ProjectID.String(),
		UserId:    m.UserID.String(),
		UserName:  m.UserName,
		Role:      FromProjectMemberRole(m.Role),
		CreatedAt: &organizationv1.Timestamp{
			Value: m.Created.Time.Format(time.RFC3339),
		},
	}
}

func FromProjectMemberRole(role string) organizationv1.ProjectMemberRole {
	switch role {
	case models.ProjectRoleAdmin:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN
	case models.ProjectRoleViewer:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER
	default:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED
	}
}

func ToProjectMemberRole(role organizationv1.ProjectMemberRole) string {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return models.ProjectRoleAdmin
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return models.ProjectRoleViewer
	default:
		return ""
	}
}
