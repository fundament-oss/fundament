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

// Ensure ProjectDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ProjectDataSource{}
var _ datasource.DataSourceWithConfigure = &ProjectDataSource{}

// ProjectDataSource defines the data source implementation.
type ProjectDataSource struct {
	client *FundamentClient
}

// NewProjectDataSource creates a new ProjectDataSource.
func NewProjectDataSource() datasource.DataSource {
	return &ProjectDataSource{}
}

// Metadata returns the data source type name.
func (d *ProjectDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

// Schema defines the schema for the data source.
func (d *ProjectDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a single project by name.",
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
				Description: "The name of the project to look up.",
				Required:    true,
			},
			"created": schema.StringAttribute{
				Description: "The timestamp when the project was created.",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ProjectDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ProjectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ProjectModel

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

	tflog.Debug(ctx, "Reading project", map[string]any{
		"name": config.Name.ValueString(),
	})

	getReq := connect.NewRequest(organizationv1.GetProjectByNameRequest_builder{
		Name: config.Name.ValueString(),
	}.Build())

	getResp, err := d.client.ProjectService.GetProjectByName(ctx, getReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project Not Found",
				fmt.Sprintf("Project with name %q does not exist.", config.Name.ValueString()),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Project Name",
				fmt.Sprintf("The project name %q is not valid: %s", config.Name.ValueString(), err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				fmt.Sprintf("You do not have permission to access project %q.", config.Name.ValueString()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Read Project",
				fmt.Sprintf("Unable to read project with name %q: %s", config.Name.ValueString(), err.Error()),
			)
		}
		return
	}

	project := getResp.Msg.GetProject()

	// Map response to state
	config.ID = types.StringValue(project.GetId())
	config.Name = types.StringValue(project.GetName())
	config.ClusterID = types.StringValue(project.GetClusterId())

	// Resolve cluster name
	clusterReq := connect.NewRequest(organizationv1.GetClusterRequest_builder{
		ClusterId: project.GetClusterId(),
	}.Build())

	clusterResp, err := d.client.ClusterService.GetCluster(ctx, clusterReq)
	if err != nil {
		tflog.Error(ctx, "Unable to resolve cluster name", map[string]any{
			"cluster_id": project.GetClusterId(),
			"error":      err.Error(),
		})
		return
	} else {
		config.ClusterName = types.StringValue(clusterResp.Msg.GetCluster().GetName())
	}

	if project.GetCreated().CheckValid() == nil {
		config.Created = types.StringValue(project.GetCreated().String())
	}

	tflog.Debug(ctx, "Read project successfully", map[string]any{
		"id":   config.ID.ValueString(),
		"name": config.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
