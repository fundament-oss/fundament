package provider

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

var _ resource.Resource = &OrganizationMemberResource{}
var _ resource.ResourceWithConfigure = &OrganizationMemberResource{}
var _ resource.ResourceWithImportState = &OrganizationMemberResource{}

type OrganizationMemberResource struct {
	client *FundamentClient
}

func NewOrganizationMemberResource() resource.Resource {
	return &OrganizationMemberResource{}
}

func (r *OrganizationMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_member"
}

func (r *OrganizationMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an organization member in Fundament. Invites a user by email and manages their permission.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the member.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				Description: "The email address of the member to invite. Changing this will destroy and recreate the member.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permission": schema.StringAttribute{
				Description: `The permission of the member. Valid values are "admin" and "viewer".`,
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("admin", "viewer"),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the member. Initially set to the email, changes when the user accepts the invite.",
				Computed:    true,
			},
			"external_id": schema.StringAttribute{
				Description: "The external identity provider ID. Set when the user authenticates.",
				Computed:    true,
			},
			"created": schema.StringAttribute{
				Description: "The timestamp when the member was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OrganizationMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*FundamentClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *FundamentClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *OrganizationMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrganizationMemberModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The Fundament client was not configured. Please report this issue to the provider developers.",
		)
		return
	}

	tflog.Debug(ctx, "Inviting organization member", map[string]any{
		"email":      plan.Email.ValueString(),
		"permission": plan.Permission.ValueString(),
	})

	inviteReq := connect.NewRequest(&organizationv1.InviteMemberRequest{
		Email:      plan.Email.ValueString(),
		Permission: plan.Permission.ValueString(),
	})

	inviteResp, err := r.client.InviteService.InviteMember(ctx, inviteReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeAlreadyExists:
			resp.Diagnostics.AddError(
				"Member Already Exists",
				fmt.Sprintf("A member with email %q already exists in this organization.", plan.Email.ValueString()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to invite members to this organization.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Invite Member",
				fmt.Sprintf("Unable to invite organization member: %s", err.Error()),
			)
		}
		return
	}

	member := inviteResp.Msg.Member
	plan.ID = types.StringValue(member.Id)
	plan.Name = types.StringValue(member.Name)

	if member.ExternalRef != nil {
		plan.ExternalID = types.StringValue(*member.ExternalRef)
	} else {
		plan.ExternalID = types.StringNull()
	}

	if member.Email != nil {
		plan.Email = types.StringValue(*member.Email)
	} else {
		plan.Email = types.StringNull()
	}

	plan.Permission = types.StringValue(member.Permission)

	if member.Created.CheckValid() == nil {
		plan.Created = types.StringValue(member.Created.String())
	} else {
		plan.Created = types.StringNull()
	}

	tflog.Info(ctx, "Invited organization member", map[string]any{
		"id":    plan.ID.ValueString(),
		"email": plan.Email.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrganizationMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrganizationMemberModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The Fundament client was not configured. Please report this issue to the provider developers.",
		)
		return
	}

	tflog.Debug(ctx, "Reading organization member", map[string]any{
		"id": state.ID.ValueString(),
	})

	member, err := r.findMemberByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Member",
			fmt.Sprintf("Unable to read organization member: %s", err.Error()),
		)
		return
	}

	if member == nil {
		tflog.Info(ctx, "Organization member not found, removing from state", map[string]any{
			"id": state.ID.ValueString(),
		})
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(member.Id)
	state.Name = types.StringValue(member.Name)
	state.Permission = types.StringValue(member.Permission)

	if member.ExternalRef != nil {
		state.ExternalID = types.StringValue(*member.ExternalRef)
	} else {
		state.ExternalID = types.StringNull()
	}

	if member.Email != nil {
		state.Email = types.StringValue(*member.Email)
	} else {
		state.Email = types.StringNull()
	}

	if member.Created.CheckValid() == nil {
		state.Created = types.StringValue(member.Created.String())
	} else {
		state.Created = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrganizationMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OrganizationMemberModel
	var state OrganizationMemberModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The Fundament client was not configured. Please report this issue to the provider developers.",
		)
		return
	}

	tflog.Debug(ctx, "Updating organization member permission", map[string]any{
		"id":             state.ID.ValueString(),
		"permission_old": state.Permission.ValueString(),
		"permission_new": plan.Permission.ValueString(),
	})

	updateReq := connect.NewRequest(&organizationv1.UpdateMemberPermissionRequest{
		Id:         state.ID.ValueString(),
		Permission: plan.Permission.ValueString(),
	})

	_, err := r.client.MemberService.UpdateMemberPermission(ctx, updateReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Member Not Found",
				fmt.Sprintf("Member %q no longer exists. It may have been deleted outside of Terraform.", state.ID.ValueString()),
			)
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Permission Update Not Allowed",
				fmt.Sprintf("Cannot update member permission: %s", err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to update member permissions in this organization.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Update Member Permission",
				fmt.Sprintf("Unable to update organization member permission: %s", err.Error()),
			)
		}
		return
	}

	// Re-read the member to get the full state
	member, err := r.findMemberByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Member",
			fmt.Sprintf("Unable to read organization member: %s", err.Error()),
		)
		return
	}

	if member == nil {
		resp.Diagnostics.AddError(
			"Member Not Found After Update",
			fmt.Sprintf("Member %q was updated but could not be read.", state.ID.ValueString()),
		)
		return
	}

	plan.ID = types.StringValue(member.Id)
	plan.Name = types.StringValue(member.Name)
	plan.Permission = types.StringValue(member.Permission)

	if member.ExternalRef != nil {
		plan.ExternalID = types.StringValue(*member.ExternalRef)
	} else {
		plan.ExternalID = types.StringNull()
	}

	if member.Email != nil {
		plan.Email = types.StringValue(*member.Email)
	} else {
		plan.Email = types.StringNull()
	}

	if member.Created.CheckValid() == nil {
		plan.Created = types.StringValue(member.Created.String())
	} else {
		plan.Created = types.StringNull()
	}

	tflog.Info(ctx, "Updated organization member permission", map[string]any{
		"id":         plan.ID.ValueString(),
		"permission": plan.Permission.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrganizationMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OrganizationMemberModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The Fundament client was not configured. Please report this issue to the provider developers.",
		)
		return
	}

	tflog.Debug(ctx, "Deleting organization member", map[string]any{
		"id": state.ID.ValueString(),
	})

	deleteReq := connect.NewRequest(&organizationv1.DeleteMemberRequest{
		Id: state.ID.ValueString(),
	})

	_, err := r.client.MemberService.DeleteMember(ctx, deleteReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			tflog.Info(ctx, "Organization member already deleted", map[string]any{
				"id": state.ID.ValueString(),
			})
			return
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Member Cannot Be Deleted",
				fmt.Sprintf("Cannot delete member: %s", err.Error()),
			)
			return
		default:
			resp.Diagnostics.AddError(
				"Unable to Delete Member",
				fmt.Sprintf("Unable to delete organization member: %s", err.Error()),
			)
			return
		}
	}

	tflog.Info(ctx, "Deleted organization member", map[string]any{
		"id": state.ID.ValueString(),
	})
}

func (r *OrganizationMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findMemberByID fetches a single member by membership ID.
// Returns the member if found, nil if not found, or an error on API failure.
func (r *OrganizationMemberResource) findMemberByID(ctx context.Context, id string) (*organizationv1.Member, error) {
	getReq := connect.NewRequest(&organizationv1.GetMemberRequest{
		Lookup: &organizationv1.GetMemberRequest_Id{Id: id},
	})

	getResp, err := r.client.MemberService.GetMember(ctx, getReq)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get member: %w", err)
	}

	return getResp.Msg.Member, nil
}
