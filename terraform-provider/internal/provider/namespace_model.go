package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

// NamespaceModel describes the namespace data model used by both the resource and data source.
type NamespaceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	ProjectID types.String `tfsdk:"project_id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	CreatedAt types.String `tfsdk:"created_at"`
}
