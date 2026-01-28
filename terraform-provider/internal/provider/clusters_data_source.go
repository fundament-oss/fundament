package provider

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure ClustersDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ClustersDataSource{}
var _ datasource.DataSourceWithConfigure = &ClustersDataSource{}

// ClustersDataSource defines the data source implementation.
type ClustersDataSource struct {
	client *FundamentClient
}

// ClustersDataSourceModel describes the data source data model.
type ClustersDataSourceModel struct {
	ID        types.String   `tfsdk:"id"`
	ProjectID types.String   `tfsdk:"project_id"`
	Clusters  []ClusterModel `tfsdk:"clusters"`
}

// ClusterModel describes a single cluster in the data source.
type ClusterModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Status types.String `tfsdk:"status"`
	Region types.String `tfsdk:"region"`
}

// NewClustersDataSource creates a new ClustersDataSource.
func NewClustersDataSource() datasource.DataSource {
	return &ClustersDataSource{}
}

// Metadata returns the data source type name.
func (d *ClustersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clusters"
}

// Schema defines the schema for the data source.
func (d *ClustersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of clusters for the current organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"project_id": schema.StringAttribute{
				Description: "Filter clusters by project ID.",
				Optional:    true,
			},
			"clusters": schema.ListNestedAttribute{
				Description: "List of clusters.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the cluster.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the cluster.",
							Computed:    true,
						},
						"status": schema.StringAttribute{
							Description: "The status of the cluster (e.g., running, provisioning, stopped).",
							Computed:    true,
						},
						"region": schema.StringAttribute{
							Description: "The region where the cluster is deployed.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ClustersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ClustersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ClustersDataSourceModel

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

	// Build the request
	projectID := state.ProjectID.ValueString()
	tflog.Debug(ctx, "Fetching clusters", map[string]any{
		"project_id": projectID,
	})

	rpcReq := connect.NewRequest(&organizationv1.ListClustersRequest{
		ProjectId: projectID,
	})

	// Call the API
	rpcResp, err := d.client.ClusterService.ListClusters(ctx, rpcReq)
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
				"You do not have permission to list clusters in this organization.",
			)
		case connect.CodeNotFound:
			if projectID != "" {
				resp.Diagnostics.AddError(
					"Project Not Found",
					fmt.Sprintf("Project with ID %q does not exist.", projectID),
				)
			} else {
				resp.Diagnostics.AddError(
					"Not Found",
					fmt.Sprintf("Unable to list clusters: %s", err.Error()),
				)
			}
		default:
			resp.Diagnostics.AddError(
				"Unable to List Clusters",
				fmt.Sprintf("Unable to list clusters: %s", err.Error()),
			)
		}
		return
	}

	// Map response to state
	state.Clusters = make([]ClusterModel, len(rpcResp.Msg.Clusters))
	for i, cluster := range rpcResp.Msg.Clusters {
		state.Clusters[i] = ClusterModel{
			ID:     types.StringValue(cluster.Id),
			Name:   types.StringValue(cluster.Name),
			Status: types.StringValue(clusterStatusToString(cluster.Status)),
			Region: types.StringValue(cluster.Region),
		}
	}

	// Set the data source ID
	state.ID = types.StringValue("clusters")
	if !state.ProjectID.IsNull() && state.ProjectID.ValueString() != "" {
		state.ID = types.StringValue("clusters-" + state.ProjectID.ValueString())
	}

	tflog.Debug(ctx, "Fetched clusters successfully", map[string]any{
		"cluster_count": len(state.Clusters),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// clusterStatusToString converts a ClusterStatus enum to a human-readable string.
func clusterStatusToString(status organizationv1.ClusterStatus) string {
	switch status {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING:
		return "provisioning"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING:
		return "starting"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING:
		return "running"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING:
		return "upgrading"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR:
		return "error"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING:
		return "stopping"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
		return "stopped"
	default:
		return "unspecified"
	}
}
