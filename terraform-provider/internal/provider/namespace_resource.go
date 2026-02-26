package provider

import (
	"context"
	"fmt"
	"time"

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

// Ensure NamespaceResource satisfies various resource interfaces.
var _ resource.Resource = &NamespaceResource{}
var _ resource.ResourceWithConfigure = &NamespaceResource{}
var _ resource.ResourceWithImportState = &NamespaceResource{}

// NamespaceResource defines the resource implementation.
type NamespaceResource struct {
	client *FundamentClient
}

// NewNamespaceResource creates a new NamespaceResource.
func NewNamespaceResource() resource.Resource {
	return &NamespaceResource{}
}

// Metadata returns the resource type name.
func (r *NamespaceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

// Schema defines the schema for the resource.
func (r *NamespaceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a namespace within a Kubernetes cluster in Fundament.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the namespace.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the namespace. Must be unique within the cluster.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The ID of the project that owns this namespace. Either project_id or project_name must be specified, but not both.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_name": schema.StringAttribute{
				Description: "The name of the project that owns this namespace. Either project_id or project_name must be specified, but not both.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster where this namespace is deployed. Derived from the project.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_name": schema.StringAttribute{
				Description: "The name of the cluster where this namespace is deployed. Derived from the project.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created": schema.StringAttribute{
				Description: "The timestamp when the namespace was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *NamespaceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// resolveProjectID resolves a project_id from the state, using project_name if project_id is not provided.
func (r *NamespaceResource) resolveProjectID(ctx context.Context, state *NamespaceResourceModel) (string, error) {
	hasProjectID := !state.ProjectID.IsNull() && !state.ProjectID.IsUnknown()
	hasProjectName := !state.ProjectName.IsNull() && !state.ProjectName.IsUnknown()

	if !hasProjectID && !hasProjectName {
		return "", fmt.Errorf("either 'project_id' or 'project_name' must be specified")
	}

	// Prefer project_id when both are present (UseStateForUnknown may populate both from state)
	if hasProjectID {
		return state.ProjectID.ValueString(), nil
	}

	// Resolve project_name to project_id
	getReq := connect.NewRequest(organizationv1.GetProjectByNameRequest_builder{
		Name: state.ProjectName.ValueString(),
	}.Build())

	getResp, err := r.client.ProjectService.GetProjectByName(ctx, getReq)
	if err != nil {
		return "", fmt.Errorf("unable to find project with name %q: %s", state.ProjectName.ValueString(), err.Error())
	}

	return getResp.Msg.GetProject().GetId(), nil
}

// populateClusterFields populates both cluster_id and cluster_name on the state from the given cluster_id.
func (r *NamespaceResource) populateClusterFields(ctx context.Context, state *NamespaceResourceModel, clusterID string) {
	state.ClusterID = types.StringValue(clusterID)

	getReq := connect.NewRequest(organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build())

	getResp, err := r.client.ClusterService.GetCluster(ctx, getReq)
	if err != nil {
		tflog.Warn(ctx, "Unable to resolve cluster name", map[string]any{
			"cluster_id": clusterID,
			"error":      err.Error(),
		})

		state.ClusterName = types.StringNull()
		return
	}

	state.ClusterName = types.StringValue(getResp.Msg.GetCluster().GetName())
}

// populateProjectFields populates both project_id and project_name on the state from the given project_id.
func (r *NamespaceResource) populateProjectFields(ctx context.Context, state *NamespaceResourceModel, projectID string) {
	state.ProjectID = types.StringValue(projectID)

	getReq := connect.NewRequest(organizationv1.GetProjectRequest_builder{
		ProjectId: projectID,
	}.Build())

	getResp, err := r.client.ProjectService.GetProject(ctx, getReq)
	if err != nil {
		tflog.Warn(ctx, "Unable to resolve project name", map[string]any{
			"project_id": projectID,
			"error":      err.Error(),
		})

		state.ProjectName = types.StringNull()
		return
	}

	state.ProjectName = types.StringValue(getResp.Msg.GetProject().GetName())
}

// Create creates a new namespace.
func (r *NamespaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NamespaceResourceModel

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

	projectID, err := r.resolveProjectID(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Resolve Project",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Creating namespace", map[string]any{
		"name":       plan.Name.ValueString(),
		"project_id": projectID,
	})

	// Create the namespace
	createReq := connect.NewRequest(organizationv1.CreateNamespaceRequest_builder{
		ProjectId: projectID,
		Name:      plan.Name.ValueString(),
	}.Build())

	createResp, err := r.client.NamespaceService.CreateNamespace(ctx, createReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Project or Cluster Not Found",
				fmt.Sprintf("The specified project or cluster does not exist: %s", err.Error()),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Namespace Configuration",
				fmt.Sprintf("Invalid namespace parameters: %s", err.Error()),
			)
		case connect.CodeAlreadyExists:
			resp.Diagnostics.AddError(
				"Namespace Already Exists",
				fmt.Sprintf("A namespace with name %q already exists in this cluster.", plan.Name.ValueString()),
			)
		case connect.CodePermissionDenied:
			resp.Diagnostics.AddError(
				"Permission Denied",
				"You do not have permission to create namespaces in this cluster.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Create Namespace",
				fmt.Sprintf("Unable to create namespace: %s", err.Error()),
			)
		}
		return
	}

	// Set the ID from the response
	plan.ID = types.StringValue(createResp.Msg.GetNamespaceId())

	// Read back the namespace to get created and other computed fields
	getReq := connect.NewRequest(organizationv1.GetNamespaceRequest_builder{
		NamespaceId: plan.ID.ValueString(),
	}.Build())

	getResp, err := r.client.NamespaceService.GetNamespace(ctx, getReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Created Namespace",
			fmt.Sprintf("Namespace was created but unable to read its details: %s", err.Error()),
		)
		return
	}

	plan.Created = types.StringValue(getResp.Msg.GetNamespace().GetCreated().AsTime().Format(time.RFC3339))
	r.populateClusterFields(ctx, &plan, getResp.Msg.GetNamespace().GetClusterId())
	r.populateProjectFields(ctx, &plan, getResp.Msg.GetNamespace().GetProjectId())

	tflog.Info(ctx, "Created namespace", map[string]any{
		"id":      plan.ID.ValueString(),
		"created": plan.Created.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NamespaceResourceModel

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

	tflog.Debug(ctx, "Reading namespace", map[string]any{
		"id": state.ID.ValueString(),
	})

	getReq := connect.NewRequest(organizationv1.GetNamespaceRequest_builder{
		NamespaceId: state.ID.ValueString(),
	}.Build())

	getResp, err := r.client.NamespaceService.GetNamespace(ctx, getReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			tflog.Info(ctx, "Namespace not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		default:
			resp.Diagnostics.AddError(
				"Unable to Read Namespace",
				fmt.Sprintf("Unable to read namespace: %s", err.Error()),
			)
			return
		}
	}

	ns := getResp.Msg.GetNamespace()
	state.Name = types.StringValue(ns.GetName())
	state.Created = types.StringValue(ns.GetCreated().AsTime().Format(time.RFC3339))
	r.populateClusterFields(ctx, &state, ns.GetClusterId())
	r.populateProjectFields(ctx, &state, ns.GetProjectId())

	tflog.Debug(ctx, "Read namespace successfully", map[string]any{
		"id":   state.ID.ValueString(),
		"name": state.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is not supported for namespaces. Any change will force replacement.
func (r *NamespaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"Namespaces cannot be updated. Any changes require replacement.",
	)
}

// Delete deletes the namespace.
func (r *NamespaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NamespaceResourceModel

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

	tflog.Debug(ctx, "Deleting namespace", map[string]any{
		"id": state.ID.ValueString(),
	})

	deleteReq := connect.NewRequest(organizationv1.DeleteNamespaceRequest_builder{
		NamespaceId: state.ID.ValueString(),
	}.Build())

	_, err := r.client.NamespaceService.DeleteNamespace(ctx, deleteReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			// Namespace already deleted, this is fine
			tflog.Info(ctx, "Namespace already deleted", map[string]any{
				"id": state.ID.ValueString(),
			})
			return
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Namespace Cannot Be Deleted",
				fmt.Sprintf("Namespace %q cannot be deleted because it has dependent resources. Delete those resources first.", state.ID.ValueString()),
			)
			return
		default:
			resp.Diagnostics.AddError(
				"Unable to Delete Namespace",
				fmt.Sprintf("Unable to delete namespace: %s", err.Error()),
			)
			return
		}
	}

	tflog.Info(ctx, "Deleted namespace", map[string]any{
		"id": state.ID.ValueString(),
	})
}

// ImportState imports an existing namespace into Terraform state.
func (r *NamespaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
