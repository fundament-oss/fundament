package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// uuidValidator validates that a string is a valid UUID using the google/uuid library.
type uuidValidator struct{}

func (v uuidValidator) Description(_ context.Context) string {
	return "value must be a valid UUID"
}

func (v uuidValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v uuidValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := uuid.Parse(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid UUID",
			fmt.Sprintf("Value %q is not a valid UUID: %s", req.ConfigValue.ValueString(), err),
		)
	}
}

// Ensure ProjectMemberResource satisfies various resource interfaces.
var _ resource.Resource = &ProjectMemberResource{}
var _ resource.ResourceWithConfigure = &ProjectMemberResource{}
var _ resource.ResourceWithImportState = &ProjectMemberResource{}

// ProjectMemberResource defines the resource implementation.
type ProjectMemberResource struct {
	client *FundamentClient
}

// NewProjectMemberResource creates a new ProjectMemberResource.
func NewProjectMemberResource() resource.Resource {
	return &ProjectMemberResource{}
}

// Metadata returns the resource type name.
func (r *ProjectMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_member"
}

// Schema defines the schema for the resource.
func (r *ProjectMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a project member in Fundament. Note: when a project is created, the authenticated user is automatically added as an admin member. This implicit member cannot be managed by this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the project member.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The ID of the project.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"user_id": schema.StringAttribute{
				Description: "The ID of the user to add as a member.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"permission": schema.StringAttribute{
				Description: "The permission of the project member.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("admin", "viewer"),
				},
			},
			"user_name": schema.StringAttribute{
				Description: "The name of the user.",
				Computed:    true,
			},
			"created": schema.StringAttribute{
				Description: "The timestamp when the member was added.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ProjectMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new project member.
func (r *ProjectMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProjectMemberModel

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

	tflog.Debug(ctx, "Creating project member", map[string]any{
		"project_id": plan.ProjectID.ValueString(),
		"user_id":    plan.UserID.ValueString(),
		"permission": plan.Permission.ValueString(),
	})

	protoRole, err := projectMemberPermissionToProto(plan.Permission.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Project Member Permission",
			fmt.Sprintf("Unable to convert permission: %s", err.Error()),
		)
		return
	}

	// Create the project member
	createReq := connect.NewRequest(&organizationv1.AddProjectMemberRequest{
		ProjectId: plan.ProjectID.ValueString(),
		UserId:    plan.UserID.ValueString(),
		Role:      protoRole,
	})

	createResp, err := r.client.ProjectService.AddProjectMember(ctx, createReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeAlreadyExists:
			resp.Diagnostics.AddError(
				"Project Member Already Exists",
				fmt.Sprintf("User %q is already a member of project %q.", plan.UserID.ValueString(), plan.ProjectID.ValueString()),
			)
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project or User Not Found",
				fmt.Sprintf("The specified project or user does not exist: %s", err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to add members to this project.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Add Project Member",
				fmt.Sprintf("Unable to add project member: %s", err.Error()),
			)
		}
		return
	}

	// Set the ID from the response
	plan.ID = types.StringValue(createResp.Msg.MemberId)

	// Read back the created member to get computed fields
	found, diags := readProjectMemberIntoModel(ctx, r.client, &plan)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Unable to Find Created Project Member",
			fmt.Sprintf("Project member was created with ID %q but could not be found.", plan.ID.ValueString()),
		)
		return
	}

	tflog.Info(ctx, "Created project member", map[string]any{
		"id":         plan.ID.ValueString(),
		"project_id": plan.ProjectID.ValueString(),
		"user_id":    plan.UserID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *ProjectMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectMemberModel

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

	tflog.Debug(ctx, "Reading project member", map[string]any{
		"id":         state.ID.ValueString(),
		"project_id": state.ProjectID.ValueString(),
	})

	found, diags := readProjectMemberIntoModel(ctx, r.client, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	if !found {
		tflog.Info(ctx, "Project member not found, removing from state", map[string]any{
			"id": state.ID.ValueString(),
		})
		resp.State.RemoveResource(ctx)
		return
	}

	tflog.Debug(ctx, "Read project member successfully", map[string]any{
		"id":         state.ID.ValueString(),
		"permission": state.Permission.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the project member role.
func (r *ProjectMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ProjectMemberModel
	var state ProjectMemberModel

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

	tflog.Debug(ctx, "Updating project member permission", map[string]any{
		"id":             state.ID.ValueString(),
		"permission_old": state.Permission.ValueString(),
		"permission_new": plan.Permission.ValueString(),
	})

	protoRole, err := projectMemberPermissionToProto(plan.Permission.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Project Member Permission",
			fmt.Sprintf("Unable to convert permission: %s", err.Error()),
		)
		return
	}

	// Update the member role
	updateReq := connect.NewRequest(&organizationv1.UpdateProjectMemberRoleRequest{
		MemberId: state.ID.ValueString(),
		Role:     protoRole,
	})

	_, err = r.client.ProjectService.UpdateProjectMemberRole(ctx, updateReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			tflog.Info(ctx, "Project member not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Project Member Update Not Allowed",
				fmt.Sprintf("Cannot update project member role: %s", err.Error()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to update member roles in this project.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Update Project Member",
				fmt.Sprintf("Unable to update project member role: %s", err.Error()),
			)
		}
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Read the member to get the updated state
	plan.ID = state.ID
	found, diags := readProjectMemberIntoModel(ctx, r.client, &plan)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Project Member Not Found After Update",
			fmt.Sprintf("Project member %q was updated but could not be found.", state.ID.ValueString()),
		)
		return
	}

	tflog.Info(ctx, "Updated project member", map[string]any{
		"id":         plan.ID.ValueString(),
		"permission": plan.Permission.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the project member.
func (r *ProjectMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectMemberModel

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

	tflog.Debug(ctx, "Removing project member", map[string]any{
		"id": state.ID.ValueString(),
	})

	deleteReq := connect.NewRequest(&organizationv1.RemoveProjectMemberRequest{
		MemberId: state.ID.ValueString(),
	})

	_, err := r.client.ProjectService.RemoveProjectMember(ctx, deleteReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			// Member already removed, this is fine
			tflog.Info(ctx, "Project member already removed", map[string]any{
				"id": state.ID.ValueString(),
			})
			return
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Project Member Cannot Be Removed",
				fmt.Sprintf("Project member %q cannot be removed: %s", state.ID.ValueString(), err.Error()),
			)
			return
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to remove members from this project.",
			)
			return
		default:
			resp.Diagnostics.AddError(
				"Unable to Remove Project Member",
				fmt.Sprintf("Unable to remove project member: %s", err.Error()),
			)
			return
		}
	}

	tflog.Info(ctx, "Removed project member", map[string]any{
		"id": state.ID.ValueString(),
	})
}

// ImportState imports an existing project member into Terraform state.
func (r *ProjectMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID should be in the format "project_id:member_id"
	// We need project_id to be able to read the member details via ListProjectMembers
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID Format",
			fmt.Sprintf(
				"Expected import ID in format 'project_id:member_id', got: %q\n\n"+
					"Example: terraform import fundament_project_member.example 01234567-89ab-cdef-0123-456789abcdef:fedcba98-7654-3210-fedc-ba9876543210",
				req.ID,
			),
		)
		return
	}

	projectID := parts[0]
	memberID := parts[1]

	tflog.Debug(ctx, "Importing project member", map[string]any{
		"project_id": projectID,
		"member_id":  memberID,
	})

	// Set the project_id and id in state
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memberID)...)
}

// readProjectMemberIntoModel fetches a project member by ID and populates the model.
// Returns (true, nil) if found, (false, nil) if not found, or (false, diags) on error.
func readProjectMemberIntoModel(ctx context.Context, client *FundamentClient, model *ProjectMemberModel) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	getReq := connect.NewRequest(&organizationv1.GetProjectMemberRequest{
		MemberId: model.ID.ValueString(),
	})

	getResp, err := client.ProjectService.GetProjectMember(ctx, getReq)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return false, diags
		}
		diags.AddError(
			"Unable to Read Project Member",
			fmt.Sprintf("Unable to read project member: %s", err.Error()),
		)
		return false, diags
	}

	member := getResp.Msg.Member
	permissionStr, err := projectMemberPermissionFromProto(member.Role)
	if err != nil {
		diags.AddError(
			"Invalid Project Member Permission",
			fmt.Sprintf("Unable to convert permission for member %q: %s", member.Id, err.Error()),
		)
		return false, diags
	}

	model.ProjectID = types.StringValue(member.ProjectId)
	model.UserID = types.StringValue(member.UserId)
	model.UserName = types.StringValue(member.UserName)
	model.Permission = types.StringValue(permissionStr)
	if member.Created != nil {
		model.Created = types.StringValue(member.Created.AsTime().Format(time.RFC3339))
	} else {
		model.Created = types.StringNull()
	}

	return true, diags
}
