package authz

import (
	"fmt"

	"github.com/google/uuid"
)

// Object type prefixes for OpenFGA
const (
	TypeUser         = "user"
	TypeOrganization = "organization"
	TypeProject      = "project"
)

// Relations for OpenFGA authorization checks
const (
	RelationMember           = "member"
	RelationAdmin            = "admin"
	RelationViewer           = "viewer"
	RelationOrganization     = "organization"
	RelationCanView          = "can_view"
	RelationCanEdit          = "can_edit"
	RelationCanDelete        = "can_delete"
	RelationCanManageMembers = "can_manage_members"
)

// UserObject returns an OpenFGA user object string: "user:<id>"
func UserObject(id uuid.UUID) string {
	return fmt.Sprintf("%s:%s", TypeUser, id.String())
}

// OrganizationObject returns an OpenFGA organization object string: "organization:<id>"
func OrganizationObject(id uuid.UUID) string {
	return fmt.Sprintf("%s:%s", TypeOrganization, id.String())
}

// ProjectObject returns an OpenFGA project object string: "project:<id>"
func ProjectObject(id uuid.UUID) string {
	return fmt.Sprintf("%s:%s", TypeProject, id.String())
}
