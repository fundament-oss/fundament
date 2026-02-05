package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

// ProjectModel describes the project data model used by both the resource and data source.
type ProjectModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Created types.String `tfsdk:"created"`
}
