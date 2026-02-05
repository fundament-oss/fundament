package provider

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure ClusterNamespacesDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ClusterNamespacesDataSource{}
var _ datasource.DataSourceWithConfigure = &ClusterNamespacesDataSource{}

// ClusterNamespacesDataSource defines the data source implementation.
type ClusterNamespacesDataSource struct {
	client *FundamentClient
}

// ClusterNamespacesDataSourceModel describes the data source data model.
type ClusterNamespacesDataSourceModel struct {
	ID         types.String      `tfsdk:"id"`
	ClusterID  types.String      `tfsdk:"cluster_id"`
	Namespaces []NamespaceModel  `tfsdk:"namespaces"`
}

// NewClusterNamespacesDataSource creates a new ClusterNamespacesDataSource.
func NewClusterNamespacesDataSource() datasource.DataSource {
	return &ClusterNamespacesDataSource{}
}

// Metadata returns the data source type name.
func (d *ClusterNamespacesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_namespaces"
}

// Schema defines the schema for the data source.
func (d *ClusterNamespacesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of namespaces in a cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster to list namespaces from.",
				Required:    true,
			},
			"namespaces": schema.ListNestedAttribute{
				Description: "List of namespaces in the cluster.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the namespace.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the namespace.",
							Computed:    true,
						},
						"project_id": schema.StringAttribute{
							Description: "The ID of the project that owns this namespace.",
							Computed:    true,
						},
						"cluster_id": schema.StringAttribute{
							Description: "The ID of the cluster containing this namespace.",
							Computed:    true,
						},
						"created": schema.StringAttribute{
							Description: "The timestamp when the namespace was created.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ClusterNamespacesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*FundamentClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *FundamentClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

// Read refreshes the Terraform state with the latest data.
func (d *ClusterNamespacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ClusterNamespacesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The Fundament client was not configured. Please report this issue to the provider developers.",
		)
		return
	}

	clusterID := state.ClusterID.ValueString()
	tflog.Debug(ctx, "Fetching namespaces", map[string]any{
		"cluster_id": clusterID,
	})

	rpcReq := connect.NewRequest(&organizationv1.ListClusterNamespacesRequest{
		ClusterId: clusterID,
	})

	// Call the API
	rpcResp, err := d.client.ClusterService.ListClusterNamespaces(ctx, rpcReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Request",
				fmt.Sprintf("Invalid request parameters: %s", err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to list namespaces in this cluster.",
			)
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Cluster Not Found",
				fmt.Sprintf("Cluster with ID %q does not exist.", clusterID),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to List Namespaces",
				fmt.Sprintf("Unable to list namespaces: %s", err.Error()),
			)
		}
		return
	}

	// Map response to state
	state.Namespaces = make([]NamespaceModel, len(rpcResp.Msg.Namespaces))
	for i, ns := range rpcResp.Msg.Namespaces {
		state.Namespaces[i] = NamespaceModel{
			ID:        types.StringValue(ns.Id),
			Name:      types.StringValue(ns.Name),
			ProjectID: types.StringValue(ns.ProjectId),
			ClusterID: types.StringValue(clusterID), // Set from request context
			Created: types.StringValue(ns.Created.AsTime().Format(time.RFC3339)),
		}
	}

	// Set the data source ID to the cluster ID
	state.ID = state.ClusterID

	tflog.Debug(ctx, "Fetched namespaces successfully", map[string]any{
		"namespace_count": len(state.Namespaces),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
