package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

// NamespaceModel describes the namespace data model used by the list data sources.
type NamespaceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	ProjectID types.String `tfsdk:"project_id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Created   types.String `tfsdk:"created"`
}

// NamespaceResourceModel describes the namespace data model used by the resource.
type NamespaceResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ProjectID   types.String `tfsdk:"project_id"`
	ProjectName types.String `tfsdk:"project_name"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	ClusterName types.String `tfsdk:"cluster_name"`
	Created     types.String `tfsdk:"created"`
}

// NamespaceDataSourceModel describes the namespace data model used by the single-namespace data source.
type NamespaceDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ClusterName types.String `tfsdk:"cluster_name"`
	ProjectName types.String `tfsdk:"project_name"`
	ProjectID   types.String `tfsdk:"project_id"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	Created     types.String `tfsdk:"created"`
}
