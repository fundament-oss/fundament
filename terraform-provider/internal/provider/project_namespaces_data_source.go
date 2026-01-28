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

// Ensure ProjectNamespacesDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ProjectNamespacesDataSource{}
var _ datasource.DataSourceWithConfigure = &ProjectNamespacesDataSource{}

// ProjectNamespacesDataSource defines the data source implementation.
type ProjectNamespacesDataSource struct {
	client *FundamentClient
}

// ProjectNamespacesDataSourceModel describes the data source data model.
type ProjectNamespacesDataSourceModel struct {
	ID         types.String                `tfsdk:"id"`
	ProjectID  types.String                `tfsdk:"project_id"`
	Namespaces []ProjectNamespaceModel     `tfsdk:"namespaces"`
}

// ProjectNamespaceModel describes a single namespace in the project data source.
type ProjectNamespaceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	ClusterID types.String `tfsdk:"cluster_id"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// NewProjectNamespacesDataSource creates a new ProjectNamespacesDataSource.
func NewProjectNamespacesDataSource() datasource.DataSource {
	return &ProjectNamespacesDataSource{}
}

// Metadata returns the data source type name.
func (d *ProjectNamespacesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_namespaces"
}

// Schema defines the schema for the data source.
func (d *ProjectNamespacesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of namespaces belonging to a project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"project_id": schema.StringAttribute{
				Description: "The ID of the project to list namespaces from.",
				Required:    true,
			},
			"namespaces": schema.ListNestedAttribute{
				Description: "List of namespaces in the project.",
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
						"cluster_id": schema.StringAttribute{
							Description: "The ID of the cluster where this namespace is deployed.",
							Computed:    true,
						},
						"created_at": schema.StringAttribute{
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
func (d *ProjectNamespacesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ProjectNamespacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ProjectNamespacesDataSourceModel

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

	projectID := state.ProjectID.ValueString()
	tflog.Debug(ctx, "Fetching project namespaces", map[string]any{
		"project_id": projectID,
	})

	rpcReq := connect.NewRequest(&organizationv1.ListProjectNamespacesRequest{
		ProjectId: projectID,
	})

	// Call the API
	rpcResp, err := d.client.ProjectService.ListProjectNamespaces(ctx, rpcReq)
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
				"You do not have permission to list namespaces in this project.",
			)
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project Not Found",
				fmt.Sprintf("Project with ID %q does not exist.", projectID),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to List Project Namespaces",
				fmt.Sprintf("Unable to list project namespaces: %s", err.Error()),
			)
		}
		return
	}

	// Map response to state
	state.Namespaces = make([]ProjectNamespaceModel, len(rpcResp.Msg.Namespaces))
	for i, ns := range rpcResp.Msg.Namespaces {
		state.Namespaces[i] = ProjectNamespaceModel{
			ID:        types.StringValue(ns.Id),
			Name:      types.StringValue(ns.Name),
			ClusterID: types.StringValue(ns.ClusterId),
			CreatedAt: types.StringValue(ns.CreatedAt.Value),
		}
	}

	// Set the data source ID
	state.ID = types.StringValue("project-namespaces-" + projectID)

	tflog.Debug(ctx, "Fetched project namespaces successfully", map[string]any{
		"namespace_count": len(state.Namespaces),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
