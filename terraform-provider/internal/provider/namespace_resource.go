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
				Description: "The ID of the project that owns this namespace.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster where this namespace will be created.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
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

// Create creates a new namespace.
func (r *NamespaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NamespaceModel

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

	tflog.Debug(ctx, "Creating namespace", map[string]any{
		"name":       plan.Name.ValueString(),
		"project_id": plan.ProjectID.ValueString(),
		"cluster_id": plan.ClusterID.ValueString(),
	})

	// Create the namespace
	createReq := connect.NewRequest(&organizationv1.CreateNamespaceRequest{
		ProjectId: plan.ProjectID.ValueString(),
		ClusterId: plan.ClusterID.ValueString(),
		Name:      plan.Name.ValueString(),
	})

	createResp, err := r.client.ClusterService.CreateNamespace(ctx, createReq)
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
	plan.ID = types.StringValue(createResp.Msg.NamespaceId)

	// Read back the namespace to get created_at and other computed fields
	// We need to list namespaces in the cluster to get the full details
	listReq := connect.NewRequest(&organizationv1.ListClusterNamespacesRequest{
		ClusterId: plan.ClusterID.ValueString(),
	})

	listResp, err := r.client.ClusterService.ListClusterNamespaces(ctx, listReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Created Namespace",
			fmt.Sprintf("Namespace was created but unable to read its details: %s", err.Error()),
		)
		return
	}

	// Find the created namespace in the list
	var found bool
	for _, ns := range listResp.Msg.Namespaces {
		if ns.Id == plan.ID.ValueString() {
			plan.CreatedAt = types.StringValue(ns.CreatedAt.Value)
			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.AddError(
			"Unable to Find Created Namespace",
			fmt.Sprintf("Namespace was created with ID %q but could not be found in the cluster.", plan.ID.ValueString()),
		)
		return
	}

	tflog.Info(ctx, "Created namespace", map[string]any{
		"id":         plan.ID.ValueString(),
		"created_at": plan.CreatedAt.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NamespaceModel

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
		"id":         state.ID.ValueString(),
		"cluster_id": state.ClusterID.ValueString(),
	})

	// List namespaces in the cluster and find this one
	listReq := connect.NewRequest(&organizationv1.ListClusterNamespacesRequest{
		ClusterId: state.ClusterID.ValueString(),
	})

	listResp, err := r.client.ClusterService.ListClusterNamespaces(ctx, listReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			tflog.Info(ctx, "Cluster not found, removing namespace from state", map[string]any{
				"id":         state.ID.ValueString(),
				"cluster_id": state.ClusterID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		default:
			resp.Diagnostics.AddError(
				"Unable to Read Namespace",
				fmt.Sprintf("Unable to list namespaces in cluster: %s", err.Error()),
			)
			return
		}
	}

	// Find this namespace in the list
	var found bool
	for _, ns := range listResp.Msg.Namespaces {
		if ns.Id == state.ID.ValueString() {
			state.Name = types.StringValue(ns.Name)
			state.ProjectID = types.StringValue(ns.ProjectId)
			state.CreatedAt = types.StringValue(ns.CreatedAt.Value)
			found = true
			break
		}
	}

	if !found {
		tflog.Info(ctx, "Namespace not found, removing from state", map[string]any{
			"id": state.ID.ValueString(),
		})
		resp.State.RemoveResource(ctx)
		return
	}

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
	var state NamespaceModel

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

	deleteReq := connect.NewRequest(&organizationv1.DeleteNamespaceRequest{
		NamespaceId: state.ID.ValueString(),
	})

	_, err := r.client.ClusterService.DeleteNamespace(ctx, deleteReq)
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
	// The import ID should be in the format "cluster_id:namespace_id"
	// We need cluster_id to be able to read the namespace details
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	// Note: The imported namespace will need cluster_id to be readable
	// Users will need to manually set cluster_id in their configuration after import
	resp.Diagnostics.AddWarning(
		"Import Requires Configuration",
		"After importing, you must add the cluster_id to your configuration for Terraform to manage this namespace properly.",
	)
}
