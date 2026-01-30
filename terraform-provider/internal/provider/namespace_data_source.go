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

// Ensure NamespaceDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &NamespaceDataSource{}
var _ datasource.DataSourceWithConfigure = &NamespaceDataSource{}

// NamespaceDataSource defines the data source implementation.
type NamespaceDataSource struct {
	client *FundamentClient
}


// NewNamespaceDataSource creates a new NamespaceDataSource.
func NewNamespaceDataSource() datasource.DataSource {
	return &NamespaceDataSource{}
}

// Metadata returns the data source type name.
func (d *NamespaceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

// Schema defines the schema for the data source.
func (d *NamespaceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a single namespace by ID within a cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the namespace to look up.",
				Required:    true,
			},
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster containing the namespace.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the namespace.",
				Computed:    true,
			},
			"project_id": schema.StringAttribute{
				Description: "The ID of the project that owns this namespace.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The timestamp when the namespace was created.",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *NamespaceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *NamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NamespaceModel

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

	tflog.Debug(ctx, "Reading namespace", map[string]any{
		"id":         config.ID.ValueString(),
		"cluster_id": config.ClusterID.ValueString(),
	})

	// Since there's no direct GetNamespace API, we list all namespaces in the cluster
	// and filter for the one we want
	listReq := connect.NewRequest(&organizationv1.ListClusterNamespacesRequest{
		ClusterId: config.ClusterID.ValueString(),
	})

	listResp, err := d.client.ClusterService.ListClusterNamespaces(ctx, listReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Cluster Not Found",
				fmt.Sprintf("Cluster with ID %q does not exist.", config.ClusterID.ValueString()),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Cluster ID",
				fmt.Sprintf("The cluster ID %q is not valid: %s", config.ClusterID.ValueString(), err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				fmt.Sprintf("You do not have permission to access cluster %q.", config.ClusterID.ValueString()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to List Namespaces",
				fmt.Sprintf("Unable to list namespaces in cluster %q: %s", config.ClusterID.ValueString(), err.Error()),
			)
		}
		return
	}

	// Find the namespace with the matching ID
	var found bool
	for _, ns := range listResp.Msg.Namespaces {
		if ns.Id == config.ID.ValueString() {
			config.Name = types.StringValue(ns.Name)
			config.ProjectID = types.StringValue(ns.ProjectId)
			config.CreatedAt = types.StringValue(ns.CreatedAt.AsTime().Format(time.RFC3339))
			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.AddError(
			"Namespace Not Found",
			fmt.Sprintf("Namespace with ID %q does not exist in cluster %q.", config.ID.ValueString(), config.ClusterID.ValueString()),
		)
		return
	}

	tflog.Debug(ctx, "Read namespace successfully", map[string]any{
		"id":         config.ID.ValueString(),
		"name":       config.Name.ValueString(),
		"project_id": config.ProjectID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
