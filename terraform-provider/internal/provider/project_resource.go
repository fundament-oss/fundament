package provider

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure ProjectResource satisfies various resource interfaces.
var _ resource.Resource = &ProjectResource{}
var _ resource.ResourceWithConfigure = &ProjectResource{}
var _ resource.ResourceWithImportState = &ProjectResource{}

// ProjectResource defines the resource implementation.
type ProjectResource struct {
	client *FundamentClient
}

// NewProjectResource creates a new ProjectResource.
func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

// Metadata returns the resource type name.
func (r *ProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

// Schema defines the schema for the resource.
func (r *ProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a project in Fundament.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the project.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the project. Can be updated to rename the project.",
				Required:    true,
			},
			"created": schema.StringAttribute{
				Description: "The timestamp when the project was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new project.
func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state ProjectModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
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

	tflog.Debug(ctx, "Creating project", map[string]any{
		"name": state.Name.ValueString(),
	})

	// Create the project
	createReq := connect.NewRequest(&organizationv1.CreateProjectRequest{
		Name: state.Name.ValueString(),
	})

	createResp, err := r.client.ProjectService.CreateProject(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Project",
			fmt.Sprintf("Unable to create project: %s", err.Error()),
		)
		return
	}

	// Set the ID from the response
	state.ID = types.StringValue(createResp.Msg.ProjectId)

	// Read the project to get the full state including created
	getReq := connect.NewRequest(&organizationv1.GetProjectRequest{
		ProjectId: createResp.Msg.ProjectId,
	})

	getResp, err := r.client.ProjectService.GetProject(ctx, getReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Created Project",
			fmt.Sprintf("Unable to read created project: %s", err.Error()),
		)
		return
	}

	// Map response to state
	if getResp.Msg.Project.Created.CheckValid() == nil {
		state.Created = types.StringValue(getResp.Msg.Project.Created.String())
	}

	tflog.Info(ctx, "Created project", map[string]any{
		"id": state.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectModel

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

	tflog.Debug(ctx, "Reading project", map[string]any{
		"id": state.ID.ValueString(),
	})

	getReq := connect.NewRequest(&organizationv1.GetProjectRequest{
		ProjectId: state.ID.ValueString(),
	})

	getResp, err := r.client.ProjectService.GetProject(ctx, getReq)
	if err != nil {
		// Check if the project was deleted (not found)
		if connect.CodeOf(err) == connect.CodeNotFound {
			tflog.Info(ctx, "Project not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Unable to Read Project",
			fmt.Sprintf("Unable to read project: %s", err.Error()),
		)
		return
	}

	project := getResp.Msg.Project

	// Map response to state
	state.ID = types.StringValue(project.Id)
	state.Name = types.StringValue(project.Name)

	if project.Created.CheckValid() == nil {
		state.Created = types.StringValue(project.Created.String())
	}

	tflog.Debug(ctx, "Read project successfully", map[string]any{
		"id": state.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the project configuration.
func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ProjectModel
	var state ProjectModel

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

	tflog.Debug(ctx, "Updating project", map[string]any{
		"id":       state.ID.ValueString(),
		"name_old": state.Name.ValueString(),
		"name_new": plan.Name.ValueString(),
	})

	// Update the project name
	updateReq := connect.NewRequest(&organizationv1.UpdateProjectRequest{
		ProjectId: state.ID.ValueString(),
		Name:      new(plan.Name.ValueString()),
	})

	_, err := r.client.ProjectService.UpdateProject(ctx, updateReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project Not Found",
				fmt.Sprintf("Project %q no longer exists. It may have been deleted outside of Terraform.", state.ID.ValueString()),
			)
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Project Update Not Allowed",
				fmt.Sprintf("Project %q cannot be updated in its current state: %s", state.ID.ValueString(), err.Error()),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Project Configuration",
				fmt.Sprintf("Invalid update parameters: %s", err.Error()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Update Project",
				fmt.Sprintf("Unable to update project: %s", err.Error()),
			)
		}
		return
	}

	// Read the project to get the updated state
	getReq := connect.NewRequest(&organizationv1.GetProjectRequest{
		ProjectId: state.ID.ValueString(),
	})

	getResp, err := r.client.ProjectService.GetProject(ctx, getReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project Not Found After Update",
				fmt.Sprintf("Project %q was updated but could not be read. It may have been deleted.", state.ID.ValueString()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Read Updated Project",
				fmt.Sprintf("Unable to read updated project: %s", err.Error()),
			)
		}
		return
	}

	project := getResp.Msg.Project

	// Update the plan with the server response
	plan.ID = types.StringValue(project.Id)
	plan.Name = types.StringValue(project.Name)

	if project.Created.CheckValid() == nil {
		plan.Created = types.StringValue(project.Created.String())
	}

	tflog.Info(ctx, "Updated project", map[string]any{
		"id": plan.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the project.
func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectModel

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

	tflog.Debug(ctx, "Deleting project", map[string]any{
		"id": state.ID.ValueString(),
	})

	deleteReq := connect.NewRequest(&organizationv1.DeleteProjectRequest{
		ProjectId: state.ID.ValueString(),
	})

	_, err := r.client.ProjectService.DeleteProject(ctx, deleteReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			// Project already deleted, this is fine
			tflog.Info(ctx, "Project already deleted", map[string]any{
				"id": state.ID.ValueString(),
			})
			return
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Project Cannot Be Deleted",
				fmt.Sprintf("Project %q cannot be deleted because it has dependent resources (e.g., namespaces). Delete those resources first.", state.ID.ValueString()),
			)
			return
		default:
			resp.Diagnostics.AddError(
				"Unable to Delete Project",
				fmt.Sprintf("Unable to delete project: %s", err.Error()),
			)
			return
		}
	}

	tflog.Info(ctx, "Deleted project", map[string]any{
		"id": state.ID.ValueString(),
	})
}

// ImportState imports an existing project into Terraform state.
func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
