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

// ProjectDataSourceModel describes the data source data model.
type ProjectDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
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
		Description: "Fetches a single project by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the project to look up.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the project.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
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
	var config ProjectDataSourceModel

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
		"id": config.ID.ValueString(),
	})

	getReq := connect.NewRequest(&organizationv1.GetProjectRequest{
		ProjectId: config.ID.ValueString(),
	})

	getResp, err := d.client.ProjectService.GetProject(ctx, getReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project Not Found",
				fmt.Sprintf("Project with ID %q does not exist.", config.ID.ValueString()),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Project ID",
				fmt.Sprintf("The project ID %q is not valid: %s", config.ID.ValueString(), err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				fmt.Sprintf("You do not have permission to access project %q.", config.ID.ValueString()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Read Project",
				fmt.Sprintf("Unable to read project with ID %q: %s", config.ID.ValueString(), err.Error()),
			)
		}
		return
	}

	project := getResp.Msg.Project

	// Map response to state
	config.ID = types.StringValue(project.Id)
	config.Name = types.StringValue(project.Name)
	config.CreatedAt = types.StringValue(project.CreatedAt.Value)

	tflog.Debug(ctx, "Read project successfully", map[string]any{
		"id":   config.ID.ValueString(),
		"name": config.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
