package provider

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure ProjectsDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ProjectsDataSource{}
var _ datasource.DataSourceWithConfigure = &ProjectsDataSource{}

// ProjectsDataSource defines the data source implementation.
type ProjectsDataSource struct {
	client *FundamentClient
}

// ProjectsDataSourceModel describes the data source data model.
type ProjectsDataSourceModel struct {
	ID        types.String   `tfsdk:"id"`
	ClusterID types.String   `tfsdk:"cluster_id"`
	Projects  []ProjectModel `tfsdk:"projects"`
}

// NewProjectsDataSource creates a new ProjectsDataSource.
func NewProjectsDataSource() datasource.DataSource {
	return &ProjectsDataSource{}
}

// Metadata returns the data source type name.
func (d *ProjectsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_projects"
}

// Schema defines the schema for the data source.
func (d *ProjectsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of projects for the current organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"cluster_id": schema.StringAttribute{
				Description: "The cluster ID to list projects for.",
				Required:    true,
			},
			"projects": schema.ListNestedAttribute{
				Description: "List of projects.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the project.",
							Computed:    true,
						},
						"cluster_id": schema.StringAttribute{
							Description: "The ID of the cluster this project belongs to.",
							Computed:    true,
						},
						"cluster_name": schema.StringAttribute{
							Description: "The name of the cluster this project belongs to.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the project.",
							Computed:    true,
						},
						"created": schema.StringAttribute{
							Description: "The timestamp when the project was created.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ProjectsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ProjectsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ProjectsDataSourceModel

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

	tflog.Debug(ctx, "Fetching projects")

	listReq := &organizationv1.ListProjectsRequest{ClusterId: state.ClusterID.ValueString()}
	rpcReq := connect.NewRequest(listReq)

	// Call the API
	rpcResp, err := d.client.ProjectService.ListProjects(ctx, rpcReq)
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
				"You do not have permission to list projects in this organization.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to List Projects",
				fmt.Sprintf("Unable to list projects: %s", err.Error()),
			)
		}
		return
	}

	// Build cluster name cache to avoid redundant API calls
	clusterNames := make(map[string]string)
	for _, project := range rpcResp.Msg.Projects {
		if _, ok := clusterNames[project.ClusterId]; !ok {
			clusterReq := connect.NewRequest(&organizationv1.GetClusterRequest{
				ClusterId: project.ClusterId,
			})
			clusterResp, err := d.client.ClusterService.GetCluster(ctx, clusterReq)
			if err == nil {
				clusterNames[project.ClusterId] = clusterResp.Msg.Cluster.Name
			}
		}
	}

	// Map response to state
	state.Projects = make([]ProjectModel, len(rpcResp.Msg.Projects))
	for i, project := range rpcResp.Msg.Projects {
		var created basetypes.StringValue

		if project.Created.CheckValid() == nil {
			created = types.StringValue(project.Created.String())
		}

		pm := ProjectModel{
			ID:        types.StringValue(project.Id),
			ClusterID: types.StringValue(project.ClusterId),
			Name:      types.StringValue(project.Name),
			Created:   created,
		}

		if name, ok := clusterNames[project.ClusterId]; ok {
			pm.ClusterName = types.StringValue(name)
		}

		state.Projects[i] = pm
	}

	// Set the data source ID
	state.ID = types.StringValue("projects")

	tflog.Debug(ctx, "Fetched projects successfully", map[string]any{
		"project_count": len(state.Projects),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
