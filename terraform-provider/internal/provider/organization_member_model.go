package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// OrganizationMemberModel describes the organization member data model used by both the resource and data source.
type OrganizationMemberModel struct {
	ID         types.String `tfsdk:"id"`
	Email      types.String `tfsdk:"email"`
	Name       types.String `tfsdk:"name"`
	ExternalID types.String `tfsdk:"external_id"`
	Role       types.String `tfsdk:"role"`
	Created    types.String `tfsdk:"created"`
}
