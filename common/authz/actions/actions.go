// Package actions maps each product action to the OpenFGA relation, object
// type and role that grant it.
package actions

import (
	"fmt"

	"github.com/google/uuid"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/common/authz"
)

const (
	ViewOrganization      = authzv1.Action_ACTION_VIEW_ORGANIZATION
	EditOrganization      = authzv1.Action_ACTION_EDIT_ORGANIZATION
	CreateAPIKey          = authzv1.Action_ACTION_CREATE_API_KEY
	ListAPIKeys           = authzv1.Action_ACTION_LIST_API_KEYS
	CreateCluster         = authzv1.Action_ACTION_CREATE_CLUSTER
	ListClusters          = authzv1.Action_ACTION_LIST_CLUSTERS
	InviteMember          = authzv1.Action_ACTION_INVITE_MEMBER
	EditMember            = authzv1.Action_ACTION_EDIT_MEMBER
	DeleteMember          = authzv1.Action_ACTION_DELETE_MEMBER
	ListMembers           = authzv1.Action_ACTION_LIST_MEMBERS
	ViewAPIKey            = authzv1.Action_ACTION_VIEW_API_KEY
	RevokeAPIKey          = authzv1.Action_ACTION_REVOKE_API_KEY
	DeleteAPIKey          = authzv1.Action_ACTION_DELETE_API_KEY
	ViewCluster           = authzv1.Action_ACTION_VIEW_CLUSTER
	EditCluster           = authzv1.Action_ACTION_EDIT_CLUSTER
	DeleteCluster         = authzv1.Action_ACTION_DELETE_CLUSTER
	ViewLogs              = authzv1.Action_ACTION_VIEW_LOGS
	CreateNodePool        = authzv1.Action_ACTION_CREATE_NODE_POOL
	ListNodePools         = authzv1.Action_ACTION_LIST_NODE_POOLS
	ListClusterNamespaces = authzv1.Action_ACTION_LIST_CLUSTER_NAMESPACES
	CreateProject         = authzv1.Action_ACTION_CREATE_PROJECT
	ListProjects          = authzv1.Action_ACTION_LIST_PROJECTS
	ViewNodePool          = authzv1.Action_ACTION_VIEW_NODE_POOL
	EditNodePool          = authzv1.Action_ACTION_EDIT_NODE_POOL
	DeleteNodePool        = authzv1.Action_ACTION_DELETE_NODE_POOL
	ViewProject           = authzv1.Action_ACTION_VIEW_PROJECT
	EditProject           = authzv1.Action_ACTION_EDIT_PROJECT
	DeleteProject         = authzv1.Action_ACTION_DELETE_PROJECT
	ManageProjectMembers  = authzv1.Action_ACTION_MANAGE_PROJECT_MEMBERS
	ListProjectMembers    = authzv1.Action_ACTION_LIST_PROJECT_MEMBERS
	CreateNamespace       = authzv1.Action_ACTION_CREATE_NAMESPACE
	ListNamespaces        = authzv1.Action_ACTION_LIST_NAMESPACES
	ViewProjectMember     = authzv1.Action_ACTION_VIEW_PROJECT_MEMBER
	EditProjectMember     = authzv1.Action_ACTION_EDIT_PROJECT_MEMBER
	DeleteProjectMember   = authzv1.Action_ACTION_DELETE_PROJECT_MEMBER
	ViewNamespace         = authzv1.Action_ACTION_VIEW_NAMESPACE
	DeleteNamespace       = authzv1.Action_ACTION_DELETE_NAMESPACE
	EditPlugin            = authzv1.Action_ACTION_EDIT_PLUGIN
)

type spec struct {
	relation authz.ActionName
	object   authz.ObjectType
	role     authzv1.RequiredRole
}

// All returns every defined action except the unspecified zero value.
func All() []authzv1.Action {
	values := authzv1.Action_ACTION_UNSPECIFIED.Descriptor().Values()
	all := make([]authzv1.Action, 0, values.Len()-1)
	for i := range values.Len() {
		if a := authzv1.Action(values.Get(i).Number()); a != authzv1.Action_ACTION_UNSPECIFIED {
			all = append(all, a)
		}
	}
	return all
}

// Relation returns the OpenFGA relation that grants a.
func Relation(a authzv1.Action) authz.Action {
	return authz.Action{Name: lookup(a).relation}
}

// Object returns the OpenFGA object a is checked against.
func Object(a authzv1.Action, id uuid.UUID) authz.Object {
	return authz.Object{Type: lookup(a).object, ID: id.String()}
}

// Role returns the role that grants a.
func Role(a authzv1.Action) authzv1.RequiredRole {
	return lookup(a).role
}

func lookup(a authzv1.Action) spec {
	switch a {
	case ViewOrganization:
		return spec{authz.ActionCanView, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case EditOrganization:
		return spec{authz.ActionCanEdit, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case CreateAPIKey:
		return spec{authz.ActionCanCreateApikey, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case ListAPIKeys:
		return spec{authz.ActionCanListApikeys, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case CreateCluster:
		return spec{authz.ActionCanCreateCluster, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case ListClusters:
		return spec{authz.ActionCanListClusters, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case InviteMember:
		return spec{authz.ActionCanInviteMember, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case EditMember:
		return spec{authz.ActionCanEditMember, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case DeleteMember:
		return spec{authz.ActionCanDeleteMember, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case ListMembers:
		return spec{authz.ActionCanListMembers, authz.ObjectTypeOrganization, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case ViewAPIKey:
		return spec{authz.ActionCanView, authz.ObjectTypeApiKey, authzv1.RequiredRole_REQUIRED_ROLE_API_KEY_OWNER}
	case RevokeAPIKey:
		return spec{authz.ActionCanEdit, authz.ObjectTypeApiKey, authzv1.RequiredRole_REQUIRED_ROLE_API_KEY_OWNER}
	case DeleteAPIKey:
		return spec{authz.ActionCanDelete, authz.ObjectTypeApiKey, authzv1.RequiredRole_REQUIRED_ROLE_API_KEY_OWNER}
	case ViewCluster:
		return spec{authz.ActionCanView, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case EditCluster:
		return spec{authz.ActionCanEdit, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case DeleteCluster:
		return spec{authz.ActionCanDelete, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case ViewLogs:
		return spec{authz.ActionCanEdit, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case CreateNodePool:
		return spec{authz.ActionCanCreateNodePool, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case ListNodePools:
		return spec{authz.ActionCanListNodePools, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case ListClusterNamespaces:
		return spec{authz.ActionCanListNamespaces, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case CreateProject:
		return spec{authz.ActionCanCreateProject, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case ListProjects:
		return spec{authz.ActionCanListProjects, authz.ObjectTypeCluster, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case ViewNodePool:
		return spec{authz.ActionCanView, authz.ObjectTypeNodePool, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER}
	case EditNodePool:
		return spec{authz.ActionCanEdit, authz.ObjectTypeNodePool, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case DeleteNodePool:
		return spec{authz.ActionCanDelete, authz.ObjectTypeNodePool, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case ViewProject:
		return spec{authz.ActionCanView, authz.ObjectTypeProject, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER}
	case EditProject:
		return spec{authz.ActionCanEdit, authz.ObjectTypeProject, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN}
	case DeleteProject:
		return spec{authz.ActionCanDelete, authz.ObjectTypeProject, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN}
	case ManageProjectMembers:
		return spec{authz.ActionCanManageMembers, authz.ObjectTypeProject, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN}
	case ListProjectMembers:
		return spec{authz.ActionCanListMembers, authz.ObjectTypeProject, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER}
	case CreateNamespace:
		return spec{authz.ActionCanCreateNamespace, authz.ObjectTypeProject, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN}
	case ListNamespaces:
		return spec{authz.ActionCanListNamespaces, authz.ObjectTypeProject, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER}
	case ViewProjectMember:
		return spec{authz.ActionCanView, authz.ObjectTypeProjectMember, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER}
	case EditProjectMember:
		return spec{authz.ActionCanEdit, authz.ObjectTypeProjectMember, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN}
	case DeleteProjectMember:
		return spec{authz.ActionCanDelete, authz.ObjectTypeProjectMember, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN}
	case ViewNamespace:
		return spec{authz.ActionCanView, authz.ObjectTypeNamespace, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER}
	case DeleteNamespace:
		return spec{authz.ActionCanDelete, authz.ObjectTypeNamespace, authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN}
	case EditPlugin:
		return spec{authz.ActionCanEdit, authz.ObjectTypePlugin, authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN}
	case authzv1.Action_ACTION_UNSPECIFIED:
		panic("actions: unspecified action")
	default:
		panic(fmt.Sprintf("actions: unmapped action %d", a))
	}
}
