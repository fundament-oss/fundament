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

// Ensure ClusterDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ClusterDataSource{}
var _ datasource.DataSourceWithConfigure = &ClusterDataSource{}

// ClusterDataSource defines the data source implementation.
type ClusterDataSource struct {
	client *FundamentClient
}

// ClusterDataSourceModel describes the data source data model.
type ClusterDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Region            types.String `tfsdk:"region"`
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`
	Status            types.String `tfsdk:"status"`
}

// NewClusterDataSource creates a new ClusterDataSource.
func NewClusterDataSource() datasource.DataSource {
	return &ClusterDataSource{}
}

// Metadata returns the data source type name.
func (d *ClusterDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

// Schema defines the schema for the data source.
func (d *ClusterDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a single Kubernetes cluster by name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the cluster.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the cluster to look up.",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "The region where the cluster is deployed.",
				Computed:    true,
			},
			"kubernetes_version": schema.StringAttribute{
				Description: "The Kubernetes version of the cluster.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of the cluster.",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ClusterDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ClusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ClusterDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
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

	tflog.Debug(ctx, "Reading cluster", map[string]any{
		"name": config.Name.ValueString(),
	})

	getReq := connect.NewRequest(&organizationv1.GetClusterByNameRequest{
		Name: config.Name.ValueString(),
	})

	getResp, err := d.client.ClusterService.GetClusterByName(ctx, getReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Cluster Not Found",
				fmt.Sprintf("Cluster with name %q does not exist.", config.Name.ValueString()),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Cluster Name",
				fmt.Sprintf("The cluster name %q is not valid: %s", config.Name.ValueString(), err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				fmt.Sprintf("You do not have permission to access cluster %q.", config.Name.ValueString()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Read Cluster",
				fmt.Sprintf("Unable to read cluster with name %q: %s", config.Name.ValueString(), err.Error()),
			)
		}
		return
	}

	cluster := getResp.Msg.Cluster

	// Map response to state
	config.ID = types.StringValue(cluster.Id)
	config.Name = types.StringValue(cluster.Name)
	config.Region = types.StringValue(cluster.Region)
	config.KubernetesVersion = types.StringValue(cluster.KubernetesVersion)
	config.Status = types.StringValue(clusterStatusToString(cluster.Status))

	tflog.Debug(ctx, "Read cluster successfully", map[string]any{
		"id":     config.ID.ValueString(),
		"name":   config.Name.ValueString(),
		"status": config.Status.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
