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

// Ensure ClusterResource satisfies various resource interfaces.
var _ resource.Resource = &ClusterResource{}
var _ resource.ResourceWithConfigure = &ClusterResource{}
var _ resource.ResourceWithImportState = &ClusterResource{}

// ClusterResource defines the resource implementation.
type ClusterResource struct {
	client *FundamentClient
}

// ClusterResourceModel describes the resource data model.
type ClusterResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Region            types.String `tfsdk:"region"`
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`
	Status            types.String `tfsdk:"status"`
}

// NewClusterResource creates a new ClusterResource.
func NewClusterResource() resource.Resource {
	return &ClusterResource{}
}

// Metadata returns the resource type name.
func (r *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

// Schema defines the schema for the resource.
func (r *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Kubernetes cluster in Fundament.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the cluster.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the cluster. Must be unique within the organization.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Description: "The region where the cluster will be deployed.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"kubernetes_version": schema.StringAttribute{
				Description: "The Kubernetes version for the cluster. Can be updated to upgrade the cluster.",
				Required:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of the cluster (e.g., provisioning, running, stopped).",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new cluster.
func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ClusterResourceModel

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

	tflog.Debug(ctx, "Creating cluster", map[string]any{
		"name":               plan.Name.ValueString(),
		"region":             plan.Region.ValueString(),
		"kubernetes_version": plan.KubernetesVersion.ValueString(),
	})

	// Create the cluster
	createReq := connect.NewRequest(&organizationv1.CreateClusterRequest{
		Name:              plan.Name.ValueString(),
		Region:            plan.Region.ValueString(),
		KubernetesVersion: plan.KubernetesVersion.ValueString(),
	})

	createResp, err := r.client.ClusterService.CreateCluster(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Cluster",
			fmt.Sprintf("Unable to create cluster: %s", err.Error()),
		)
		return
	}

	// Set the ID from the response
	plan.ID = types.StringValue(createResp.Msg.ClusterId)

	// Read the cluster to get the full state including status
	getReq := connect.NewRequest(&organizationv1.GetClusterRequest{
		ClusterId: createResp.Msg.ClusterId,
	})

	getResp, err := r.client.ClusterService.GetCluster(ctx, getReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Created Cluster",
			fmt.Sprintf("Unable to read created cluster: %s", err.Error()),
		)
		return
	}

	// Map response to state
	plan.Status = types.StringValue(clusterStatusToString(getResp.Msg.Cluster.Status))

	tflog.Info(ctx, "Created cluster", map[string]any{
		"id":     plan.ID.ValueString(),
		"status": plan.Status.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *ClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ClusterResourceModel

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

	tflog.Debug(ctx, "Reading cluster", map[string]any{
		"id": state.ID.ValueString(),
	})

	getReq := connect.NewRequest(&organizationv1.GetClusterRequest{
		ClusterId: state.ID.ValueString(),
	})

	getResp, err := r.client.ClusterService.GetCluster(ctx, getReq)
	if err != nil {
		// Check if the cluster was deleted (not found)
		// Connect errors include the code in the error message
		if connect.CodeOf(err) == connect.CodeNotFound {
			tflog.Info(ctx, "Cluster not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Unable to Read Cluster",
			fmt.Sprintf("Unable to read cluster: %s", err.Error()),
		)
		return
	}

	cluster := getResp.Msg.Cluster

	// Map response to state
	state.ID = types.StringValue(cluster.Id)
	state.Name = types.StringValue(cluster.Name)
	state.Region = types.StringValue(cluster.Region)
	state.KubernetesVersion = types.StringValue(cluster.KubernetesVersion)
	state.Status = types.StringValue(clusterStatusToString(cluster.Status))

	tflog.Debug(ctx, "Read cluster successfully", map[string]any{
		"id":     state.ID.ValueString(),
		"status": state.Status.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the cluster configuration.
func (r *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ClusterResourceModel
	var state ClusterResourceModel

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

	tflog.Debug(ctx, "Updating cluster", map[string]any{
		"id":                     state.ID.ValueString(),
		"kubernetes_version_old": state.KubernetesVersion.ValueString(),
		"kubernetes_version_new": plan.KubernetesVersion.ValueString(),
	})

	// Only kubernetes_version can be updated
	updateReq := connect.NewRequest(&organizationv1.UpdateClusterRequest{
		ClusterId:         state.ID.ValueString(),
		KubernetesVersion: new(plan.KubernetesVersion.ValueString()),
	})

	_, err := r.client.ClusterService.UpdateCluster(ctx, updateReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Cluster Not Found",
				fmt.Sprintf("Cluster %q no longer exists. It may have been deleted outside of Terraform.", state.ID.ValueString()),
			)
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Cluster Update Not Allowed",
				fmt.Sprintf("Cluster %q cannot be updated in its current state: %s", state.ID.ValueString(), err.Error()),
			)
		case connect.CodeInvalidArgument:
			resp.Diagnostics.AddError(
				"Invalid Cluster Configuration",
				fmt.Sprintf("Invalid update parameters: %s", err.Error()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Update Cluster",
				fmt.Sprintf("Unable to update cluster: %s", err.Error()),
			)
		}
		return
	}

	// Read the cluster to get the updated state
	getReq := connect.NewRequest(&organizationv1.GetClusterRequest{
		ClusterId: state.ID.ValueString(),
	})

	getResp, err := r.client.ClusterService.GetCluster(ctx, getReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			resp.Diagnostics.AddError(
				"Cluster Not Found After Update",
				fmt.Sprintf("Cluster %q was updated but could not be read. It may have been deleted.", state.ID.ValueString()),
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Read Updated Cluster",
				fmt.Sprintf("Unable to read updated cluster: %s", err.Error()),
			)
		}
		return
	}

	cluster := getResp.Msg.Cluster

	// Update the plan with the server response
	plan.ID = types.StringValue(cluster.Id)
	plan.Name = types.StringValue(cluster.Name)
	plan.Region = types.StringValue(cluster.Region)
	plan.KubernetesVersion = types.StringValue(cluster.KubernetesVersion)
	plan.Status = types.StringValue(clusterStatusToString(cluster.Status))

	tflog.Info(ctx, "Updated cluster", map[string]any{
		"id":     plan.ID.ValueString(),
		"status": plan.Status.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the cluster.
func (r *ClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ClusterResourceModel

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

	tflog.Debug(ctx, "Deleting cluster", map[string]any{
		"id": state.ID.ValueString(),
	})

	deleteReq := connect.NewRequest(&organizationv1.DeleteClusterRequest{
		ClusterId: state.ID.ValueString(),
	})

	_, err := r.client.ClusterService.DeleteCluster(ctx, deleteReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			// Cluster already deleted, this is fine
			tflog.Info(ctx, "Cluster already deleted", map[string]any{
				"id": state.ID.ValueString(),
			})
			return
		case connect.CodeFailedPrecondition:
			resp.Diagnostics.AddError(
				"Cluster Cannot Be Deleted",
				fmt.Sprintf("Cluster %q cannot be deleted because it has dependent resources (e.g., namespaces). Delete those resources first.", state.ID.ValueString()),
			)
			return
		default:
			resp.Diagnostics.AddError(
				"Unable to Delete Cluster",
				fmt.Sprintf("Unable to delete cluster: %s", err.Error()),
			)
			return
		}
	}

	tflog.Info(ctx, "Deleted cluster", map[string]any{
		"id": state.ID.ValueString(),
	})
}

// ImportState imports an existing cluster into Terraform state.
func (r *ClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
