package provider

import (
	"fmt"

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
func projectMemberRoleToProto(role string) (organizationv1.ProjectMemberRole, error) {
	switch role {
	case "admin":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN, nil
	case "viewer":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER, nil
	default:
		return 0, fmt.Errorf("unknown project member role: %q", role)
	}
}

// projectMemberRoleToString converts a proto enum value to a string role.
func projectMemberRoleToString(role organizationv1.ProjectMemberRole) (string, error) {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return "admin", nil
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return "viewer", nil
	default:
		return "", fmt.Errorf("unknown project member role proto value: %d", role)
	}
}
