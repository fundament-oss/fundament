// Package actions maps each product action to the OpenFGA relation, object type
// and role that grant it. Actions come from model.fga; see actions_gen.go.
package actions

import (
	"fmt"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
)

// Role returns the role that grants a.
func Role(a authzv1.Action) authzv1.RequiredRole {
	switch a {
	case OrganizationCanEdit, OrganizationCanCreateCluster, OrganizationCanCreatePlugin,
		OrganizationCanInviteMember, OrganizationCanEditMember, OrganizationCanDeleteMember,
		ClusterCanEdit, ClusterCanDelete, ClusterCanCreateNodePool, ClusterCanCreateProject, ClusterCanViewLogs,
		NodePoolCanEdit, NodePoolCanDelete,
		PluginCanEdit, PluginCanDelete:
		return authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN
	case OrganizationCanView, OrganizationCanCreateApikey, OrganizationCanListApikeys,
		OrganizationCanListClusters, OrganizationCanListMembers,
		ClusterCanView, ClusterCanListNodePools, ClusterCanListNamespaces, ClusterCanListProjects,
		NodePoolCanView:
		return authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER
	case ProjectCanEdit, ProjectCanDelete, ProjectCanManageMembers, ProjectCanCreateNamespace,
		ProjectMemberCanEdit, ProjectMemberCanDelete,
		NamespaceCanDelete:
		return authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN
	case ProjectCanView, ProjectCanListMembers, ProjectCanListNamespaces,
		ProjectMemberCanView,
		NamespaceCanView:
		return authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER
	case ApiKeyCanView, ApiKeyCanEdit, ApiKeyCanDelete:
		return authzv1.RequiredRole_REQUIRED_ROLE_API_KEY_OWNER
	case authzv1.Action_ACTION_UNSPECIFIED:
		panic("actions: unspecified action")
	default:
		panic(fmt.Sprintf("actions: action %d has no role", a))
	}
}
