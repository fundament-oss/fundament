package adapter

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
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
		CreatedAt: timestamppb.New(m.Created.Time),
	}
}

func FromProjectMemberRole(role dbconst.ProjectMemberRole) organizationv1.ProjectMemberRole {
	switch role {
	case dbconst.ProjectMemberRole_Admin:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN
	case dbconst.ProjectMemberRole_Viewer:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER
	default:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED
	}
}

func ToProjectMemberRole(role organizationv1.ProjectMemberRole) dbconst.ProjectMemberRole {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return dbconst.ProjectMemberRole_Admin
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return dbconst.ProjectMemberRole_Viewer
	default:
		return ""
	}
}
