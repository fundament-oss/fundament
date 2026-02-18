package provider

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

var _ datasource.DataSource = &OrganizationMembersDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationMembersDataSource{}

type OrganizationMembersDataSource struct {
	client *FundamentClient
}

type OrganizationMembersDataSourceModel struct {
	ID      types.String              `tfsdk:"id"`
	Members []OrganizationMemberModel `tfsdk:"members"`
}

func NewOrganizationMembersDataSource() datasource.DataSource {
	return &OrganizationMembersDataSource{}
}

func (d *OrganizationMembersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_members"
}

func (d *OrganizationMembersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of members for the current organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"members": schema.ListNestedAttribute{
				Description: "List of organization members.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the member.",
							Computed:    true,
						},
						"email": schema.StringAttribute{
							Description: "The email address of the member.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The display name of the member.",
							Computed:    true,
						},
						"external_id": schema.StringAttribute{
							Description: "The external identity provider ID.",
							Computed:    true,
						},
						"role": schema.StringAttribute{
							Description: "The role of the member.",
							Computed:    true,
						},
						"created": schema.StringAttribute{
							Description: "The timestamp when the member was created.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *OrganizationMembersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OrganizationMembersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state OrganizationMembersDataSourceModel

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

	tflog.Debug(ctx, "Fetching organization members")

	rpcReq := connect.NewRequest(&organizationv1.ListMembersRequest{})

	rpcResp, err := d.client.MemberService.ListMembers(ctx, rpcReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to list members in this organization.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to List Members",
				fmt.Sprintf("Unable to list organization members: %s", err.Error()),
			)
		}
		return
	}

	state.Members = make([]OrganizationMemberModel, len(rpcResp.Msg.Members))
	for i, member := range rpcResp.Msg.Members {
		m := OrganizationMemberModel{
			ID:   types.StringValue(member.Id),
			Name: types.StringValue(member.Name),
			Role: types.StringValue(member.Permission),
		}

		if member.Email != nil {
			m.Email = types.StringValue(*member.Email)
		} else {
			m.Email = types.StringNull()
		}

		if member.ExternalRef != nil {
			m.ExternalID = types.StringValue(*member.ExternalRef)
		} else {
			m.ExternalID = types.StringNull()
		}

		if member.Created.CheckValid() == nil {
			m.Created = types.StringValue(member.Created.String())
		}

		state.Members[i] = m
	}

	state.ID = types.StringValue("organization_members")

	tflog.Debug(ctx, "Fetched organization members successfully", map[string]any{
		"member_count": len(state.Members),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
