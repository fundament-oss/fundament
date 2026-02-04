package organization

import (
	"github.com/fundament-oss/fundament/common/dbconst"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func projectMemberRoleFromDB(role dbconst.ProjectMemberRole) organizationv1.ProjectMemberRole {
	switch role {
	case dbconst.ProjectMemberRole_Admin:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN
	case dbconst.ProjectMemberRole_Viewer:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER
	default:
		panic("unknown dbconst project member role")
	}
}

func projectMemberRoleToDB(role organizationv1.ProjectMemberRole) dbconst.ProjectMemberRole {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return dbconst.ProjectMemberRole_Admin
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return dbconst.ProjectMemberRole_Viewer
	default:
		panic("unknown proto project member role")
	}
}
