package provider

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// Ensure ProjectMembersDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ProjectMembersDataSource{}
var _ datasource.DataSourceWithConfigure = &ProjectMembersDataSource{}

// ProjectMembersDataSource defines the data source implementation.
type ProjectMembersDataSource struct {
	client *FundamentClient
}

// ProjectMembersDataSourceModel describes the data source data model.
type ProjectMembersDataSourceModel struct {
	ProjectID types.String         `tfsdk:"project_id"`
	Members   []ProjectMemberModel `tfsdk:"members"`
}

// NewProjectMembersDataSource creates a new ProjectMembersDataSource.
func NewProjectMembersDataSource() datasource.DataSource {
	return &ProjectMembersDataSource{}
}

// Metadata returns the data source type name.
func (d *ProjectMembersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_members"
}

// Schema defines the schema for the data source.
func (d *ProjectMembersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of members for a project.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Description: "The ID of the project to list members for.",
				Required:    true,
			},
			"members": schema.ListNestedAttribute{
				Description: "List of project members.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the project member.",
							Computed:    true,
						},
						"project_id": schema.StringAttribute{
							Description: "The ID of the project.",
							Computed:    true,
						},
						"user_id": schema.StringAttribute{
							Description: "The ID of the user.",
							Computed:    true,
						},
						"user_name": schema.StringAttribute{
							Description: "The name of the user.",
							Computed:    true,
						},
						"role": schema.StringAttribute{
							Description: "The role of the project member.",
							Computed:    true,
						},
						"created": schema.StringAttribute{
							Description: "The timestamp when the member was added.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ProjectMembersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ProjectMembersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ProjectMembersDataSourceModel

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

	tflog.Debug(ctx, "Fetching project members", map[string]any{
		"project_id": state.ProjectID.ValueString(),
	})

	rpcReq := connect.NewRequest(&organizationv1.ListProjectMembersRequest{
		ProjectId: state.ProjectID.ValueString(),
	})

	rpcResp, err := d.client.ProjectService.ListProjectMembers(ctx, rpcReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project Not Found",
				fmt.Sprintf("Project %q does not exist.", state.ProjectID.ValueString()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to list members of this project.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to List Project Members",
				fmt.Sprintf("Unable to list project members: %s", err.Error()),
			)
		}
		return
	}

	// Map response to state
	state.Members = make([]ProjectMemberModel, len(rpcResp.Msg.Members))
	for i, member := range rpcResp.Msg.Members {
		var created types.String
		if member.Created != nil {
			created = types.StringValue(member.Created.AsTime().Format(time.RFC3339))
		}

		state.Members[i] = ProjectMemberModel{
			ID:        types.StringValue(member.Id),
			ProjectID: types.StringValue(member.ProjectId),
			UserID:    types.StringValue(member.UserId),
			UserName:  types.StringValue(member.UserName),
			Role:      types.StringValue(projectMemberRoleToString(member.Role)),
			Created:   created,
		}
	}

	tflog.Debug(ctx, "Fetched project members successfully", map[string]any{
		"project_id":   state.ProjectID.ValueString(),
		"member_count": len(state.Members),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
