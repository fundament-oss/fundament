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
		Description: "Fetches a single namespace by cluster name, project name, and namespace name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the namespace.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the namespace to look up.",
				Required:    true,
			},
			"cluster_name": schema.StringAttribute{
				Description: "The name of the cluster containing the namespace.",
				Required:    true,
			},
			"project_name": schema.StringAttribute{
				Description: "The name of the project owning the namespace.",
				Required:    true,
			},
			"project_id": schema.StringAttribute{
				Description: "The ID of the project that owns this namespace.",
				Computed:    true,
			},
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster containing the namespace.",
				Computed:    true,
			},
			"created": schema.StringAttribute{
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
	var config NamespaceDataSourceModel

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

	clusterName := config.ClusterName.ValueString()
	projectName := config.ProjectName.ValueString()
	namespaceName := config.Name.ValueString()

	tflog.Debug(ctx, "Reading namespace by cluster, project, and name", map[string]any{
		"cluster_name":   clusterName,
		"project_name":   projectName,
		"namespace_name": namespaceName,
	})

	rpcReq := connect.NewRequest(&organizationv1.GetNamespaceByProjectAndNameRequest{
		ClusterName:   clusterName,
		ProjectName:   projectName,
		NamespaceName: namespaceName,
	})

	rpcResp, err := d.client.NamespaceService.GetNamespaceByProjectAndName(ctx, rpcReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Namespace Not Found",
				fmt.Sprintf("Namespace %q does not exist in project %q of cluster %q.", namespaceName, projectName, clusterName),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Request",
				fmt.Sprintf("Invalid request parameters: %s", err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to access this namespace.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Get Namespace",
				fmt.Sprintf("Unable to get namespace: %s", err.Error()),
			)
		}
		return
	}

	ns := rpcResp.Msg.Namespace
	config.ID = types.StringValue(ns.Id)
	config.Name = types.StringValue(ns.Name)
	config.ProjectID = types.StringValue(ns.ProjectId)
	config.ClusterID = types.StringValue(ns.ClusterId)
	config.Created = types.StringValue(ns.Created.String())

	tflog.Debug(ctx, "Read namespace successfully", map[string]any{
		"id":         config.ID.ValueString(),
		"name":       config.Name.ValueString(),
		"project_id": config.ProjectID.ValueString(),
		"cluster_id": config.ClusterID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
