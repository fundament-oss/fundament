package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ProjectMemberModel describes the project member data model used by both the resource and data source.
type ProjectMemberModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	UserID     types.String `tfsdk:"user_id"`
	UserName   types.String `tfsdk:"user_name"`
	Email      types.String `tfsdk:"email"`
	Permission types.String `tfsdk:"permission"`
	Created    types.String `tfsdk:"created"`
}

// projectMemberEmail returns the member's email address, or null when the
// identity provider sent none.
func projectMemberEmail(member *organizationv1.ProjectMember) types.String {
	if member.HasEmail() {
		return types.StringValue(member.GetEmail())
	}
	return types.StringNull()
}

// projectMemberPermissionToProto converts a string permission to the proto enum value.
func projectMemberPermissionToProto(permission string) (organizationv1.ProjectMemberRole, error) {
	switch permission {
	case "admin":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN, nil
	case "viewer":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER, nil
	default:
		return 0, fmt.Errorf("unknown project member permission: %q", permission)
	}
}

// projectMemberPermissionFromProto converts a proto enum value to a string permission.
func projectMemberPermissionFromProto(role organizationv1.ProjectMemberRole) (string, error) {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return "admin", nil
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return "viewer", nil
	default:
		return "", fmt.Errorf("unknown project member permission proto value: %d", role)
	}
}
