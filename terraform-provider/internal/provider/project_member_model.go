package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ProjectMemberModel describes the project member data model used by both the resource and data source.
type ProjectMemberModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	UserID    types.String `tfsdk:"user_id"`
	UserName  types.String `tfsdk:"user_name"`
	Role      types.String `tfsdk:"role"`
	Created   types.String `tfsdk:"created"`
}

// projectMemberRoleToProto converts a string role to the proto enum value.
func projectMemberRoleToProto(role string) organizationv1.ProjectMemberRole {
	switch role {
	case "admin":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN
	case "viewer":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER
	default:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED
	}
}

// projectMemberRoleToString converts a proto enum value to a string role.
func projectMemberRoleToString(role organizationv1.ProjectMemberRole) string {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return "admin"
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return "viewer"
	default:
		return ""
	}
}
