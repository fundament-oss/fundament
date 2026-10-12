package provider

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
var _ resource.ResourceWithModifyPlan = &ClusterResource{}

// ClusterResource defines the resource implementation.
type ClusterResource struct {
	client *FundamentClient
}

// ClusterResourceModel describes the resource data model.
type ClusterResourceModel struct {
	ID                types.String           `tfsdk:"id"`
	Name              types.String           `tfsdk:"name"`
	Region            types.String           `tfsdk:"region"`
	KubernetesVersion types.String           `tfsdk:"kubernetes_version"`
	Status            types.String           `tfsdk:"status"`
	NodePools         []ClusterNodePoolModel `tfsdk:"node_pool"`
	Timeouts          timeouts.Value         `tfsdk:"timeouts"`
}

const (
	defaultClusterCreateTimeout = 30 * time.Minute
	defaultClusterUpdateTimeout = 30 * time.Minute
	defaultClusterDeleteTimeout = 30 * time.Minute
)

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
		Description: "Manages a Kubernetes cluster in Fundament.\n\n" +
			"A create waits until Gardener has built the cluster and its nodes; an update until Gardener has rolled out " +
			"the change, such as a Kubernetes upgrade; a destroy until Gardener has removed the cluster. " +
			"An apply fails when Fundament gives up applying a change, for example because Gardener rejects it, or when the " +
			"cluster does not run within the timeout.\n\n" +
			"When a create fails after Fundament has accepted the cluster, OpenTofu marks it tainted and the next apply deletes and recreates it. " +
			"If only the wait failed (a timeout, an interruption, a lost connection), Fundament keeps building it: run `tofu untaint` on its address " +
			"to keep it, and set `timeouts { create = ... }` to wait longer next time. A cluster whose deletion did not finish " +
			"stays in the state, and the next refresh shows it as deleting: the next destroy waits for it, and the next apply recreates it once it is gone.",
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
				Description: "The current status of the cluster: provisioning, running, upgrading (Gardener rolls out a change; the cluster stays usable), error (Gardener reports a problem, which it usually retries), deleting or stopped.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"node_pool": clusterNodePoolBlock(),
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create:            true,
				Update:            true,
				Delete:            true,
				CreateDescription: "How long to wait for a new cluster to run (default 30m).",
				UpdateDescription: "How long to wait for a change to be rolled out (default 30m).",
				DeleteDescription: "How long to wait for the cluster to be gone (default 30m).",
			}),
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
	createReq := organizationv1.CreateClusterRequest_builder{
		Name:              plan.Name.ValueString(),
		Region:            plan.Region.ValueString(),
		KubernetesVersion: plan.KubernetesVersion.ValueString(),
	}.Build()

	// Check the pools first: a pool the API refuses after the cluster exists
	// would leave a tainted cluster, which the next apply replaces.
	resp.Diagnostics.Append(checkMachineTypes(ctx, r.client, plan.Region.ValueString(), plan.NodePools)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createResp, err := createIdempotent(ctx, r.client.ClusterService.CreateCluster, createReq)
	if err != nil {
		detail := fmt.Sprintf("Unable to create cluster: %s.", err.Error())
		if connect.CodeOf(err) == connect.CodeAlreadyExists {
			detail += r.existingClusterHint(ctx, plan.Name.ValueString())
		}
		resp.Diagnostics.AddError("Unable to Create Cluster", detail)
		return
	}

	// Set the ID from the response
	plan.ID = types.StringValue(createResp.GetClusterId())
	plan.Status = types.StringNull()

	// Record the cluster now: if a later step fails, the apply marks it
	// tainted instead of leaving it unmanaged.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// cluster-worker may already have given the Shoot a default worker pool;
	// created right away, the pools replace it before Gardener builds workers.
	// A new cluster has no pools, so there is nothing to list; ListNodePools
	// could also be refused until the new cluster's permissions are synced.
	resp.Diagnostics.Append(applyNodePoolChanges(ctx, r.client, plan.ID.ValueString(), plan.Region.ValueString(), nil, plan.NodePools)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, defaultClusterCreateTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	waitCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	cluster, err := waitForClusterRunning(waitCtx, r.client, plan.ID.ValueString())
	if err != nil {
		detail := fmt.Sprintf("Cluster %q: %s.", plan.Name.ValueString(), err)
		if waitInconclusive(err) {
			detail += " Fundament keeps building it, but OpenTofu has marked it tainted, so the next apply deletes and " +
				"recreates it. To keep this cluster instead, run `tofu untaint` on its address: Fundament finishes it, " +
				"and `tofu plan` shows its status. Set `timeouts { create = ... }` to wait longer next time."
		}
		resp.Diagnostics.AddError("Cluster Did Not Start", detail)
		return
	}

	plan.Status = types.StringValue(clusterStatusToString(cluster.GetStatus()))

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

	getReq := organizationv1.GetClusterRequest_builder{
		ClusterId: state.ID.ValueString(),
	}.Build()

	getResp, err := r.client.ClusterService.GetCluster(ctx, getReq)
	if err != nil {
		// Check if the cluster was deleted (not found)
		// Connect errors include the code in the error message
		// The API answers both for a cluster whose deletion was requested.
		if code := connect.CodeOf(err); code == connect.CodeNotFound || code == connect.CodePermissionDenied {
			r.readMissingCluster(ctx, &state, err, resp)
			return
		}

		resp.Diagnostics.AddError(
			"Unable to Read Cluster",
			fmt.Sprintf("Unable to read cluster: %s", err.Error()),
		)
		return
	}

	cluster := getResp.GetCluster()

	// Map response to state
	state.ID = types.StringValue(cluster.GetId())
	state.Name = types.StringValue(cluster.GetName())
	state.Region = types.StringValue(cluster.GetRegion())
	state.KubernetesVersion = types.StringValue(cluster.GetKubernetesVersion())
	state.Status = types.StringValue(clusterStatusToString(cluster.GetStatus()))

	nodePools, err := listNodePools(ctx, r.client, state.ID.ValueString())
	if err != nil {
		// Deleted since the GetCluster above.
		if connect.CodeOf(err) == connect.CodeNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Node Pools", fmt.Sprintf("Cluster %q: %s", state.ID.ValueString(), err))
		return
	}
	state.NodePools = orderNodePools(nodePools, state.NodePools)

	tflog.Debug(ctx, "Read cluster successfully", map[string]any{
		"id":     state.ID.ValueString(),
		"status": state.Status.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// existingClusterHint tells how to take over a cluster that already exists
// under this name, for example one an apply created before OpenTofu was
// killed without saving its state.
func (r *ClusterResource) existingClusterHint(ctx context.Context, name string) string {
	resp, err := r.client.ClusterService.GetClusterByName(ctx, organizationv1.GetClusterByNameRequest_builder{Name: name}.Build())
	if err != nil {
		return " If an earlier apply created it and was stopped before saving it, take it over with `tofu import` and the cluster's ID."
	}
	return fmt.Sprintf(" If an earlier apply created it and was stopped before saving it, take it over with: "+
		"tofu import <address of this resource> %s", resp.GetCluster().GetId())
}

// readMissingCluster handles a cluster GetCluster no longer returns. That
// happens as soon as its deletion is requested, while Gardener still removes
// it: such a cluster stays in state as deleting, so the next destroy waits
// for it and the next apply replaces it (see ModifyPlan). A cluster that is
// gone leaves the state. A cluster listed with another status was refused
// for another reason, such as a lost permission, which getErr reports.
func (r *ClusterResource) readMissingCluster(ctx context.Context, state *ClusterResourceModel, getErr error, resp *resource.ReadResponse) {
	listed, err := r.listedCluster(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Cluster", err.Error())
		return
	}
	if listed == nil {
		tflog.Info(ctx, "Cluster not found, removing from state", map[string]any{"id": state.ID.ValueString()})
		resp.State.RemoveResource(ctx)
		return
	}
	if listed.GetStatus() != organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING {
		resp.Diagnostics.AddError("Unable to Read Cluster", fmt.Sprintf("Unable to read cluster %q: %s", state.Name.ValueString(), getErr))
		return
	}

	tflog.Info(ctx, "Cluster is being deleted", map[string]any{"id": state.ID.ValueString()})
	state.Status = types.StringValue(clusterStatusToString(listed.GetStatus()))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// listedCluster returns the cluster as ListClusters lists it, or nil when it
// is not listed. ListClusters keeps a cluster until Gardener has removed it.
func (r *ClusterResource) listedCluster(ctx context.Context, clusterID string) (*organizationv1.ListClustersResponse_ClusterSummary, error) {
	listResp, err := r.client.ClusterService.ListClusters(ctx, organizationv1.ListClustersRequest_builder{}.Build())
	if err != nil {
		return nil, fmt.Errorf("unable to list clusters: %w", err)
	}
	for _, c := range listResp.GetClusters() {
		if c.GetId() == clusterID {
			return c, nil
		}
	}
	return nil, nil
}

// ModifyPlan replaces a cluster that is being deleted: the apply then waits
// until it is gone and creates it again.
func (r *ClusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) { //nolint:gocritic // signature set by resource.ResourceWithModifyPlan
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var status types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("status"), &status)...)
	if status.ValueString() == clusterStatusToString(organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING) {
		// A replacement only takes effect on an attribute that changes.
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("status"), types.StringUnknown())...)
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("status"))
	}
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

	current, err := listNodePools(ctx, r.client, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Node Pools", fmt.Sprintf("Cluster %q: %s", state.Name.ValueString(), err))
		return
	}
	// Check the machine types before anything changes, so a type the region
	// does not offer fails before an upgrade has gone out.
	resp.Diagnostics.Append(checkMachineTypes(ctx, r.client, state.Region.ValueString(), planNodePoolChanges(current, plan.NodePools).Create)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.KubernetesVersion.Equal(state.KubernetesVersion) {
		r.updateKubernetesVersion(ctx, state.ID.ValueString(), plan.KubernetesVersion.ValueString(), &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(applyNodePoolChanges(ctx, r.client, state.ID.ValueString(), state.Region.ValueString(), current, plan.NodePools)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, defaultClusterUpdateTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	waitCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	cluster, err := waitForClusterRunning(waitCtx, r.client, state.ID.ValueString())
	if err != nil {
		detail := fmt.Sprintf("Cluster %q: %s.", state.Name.ValueString(), err)
		if waitInconclusive(err) {
			detail += " Fundament has the change and keeps applying it; the next plan shows what is left to do."
		}
		resp.Diagnostics.AddError("Cluster Update Did Not Complete", detail)
		return
	}

	// Update the plan with the server response
	plan.ID = types.StringValue(cluster.GetId())
	plan.Name = types.StringValue(cluster.GetName())
	plan.Region = types.StringValue(cluster.GetRegion())
	plan.KubernetesVersion = types.StringValue(cluster.GetKubernetesVersion())
	plan.Status = types.StringValue(clusterStatusToString(cluster.GetStatus()))

	tflog.Info(ctx, "Updated cluster", map[string]any{
		"id":     plan.ID.ValueString(),
		"status": plan.Status.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// updateKubernetesVersion upgrades the cluster to version.
func (r *ClusterResource) updateKubernetesVersion(ctx context.Context, clusterID, version string, diags *diag.Diagnostics) {
	updateReq := organizationv1.UpdateClusterRequest_builder{
		ClusterId:         clusterID,
		KubernetesVersion: &version,
	}.Build()

	_, err := r.client.ClusterService.UpdateCluster(ctx, updateReq)
	if err == nil {
		return
	}

	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		diags.AddError(
			"Cluster Not Found",
			fmt.Sprintf("Cluster %q no longer exists. It may have been deleted outside of Terraform.", clusterID),
		)
	case connect.CodeFailedPrecondition:
		diags.AddError(
			"Cluster Update Not Allowed",
			fmt.Sprintf("Cluster %q cannot be updated in its current state: %s", clusterID, err.Error()),
		)
	case connect.CodeInvalidArgument:
		diags.AddError(
			"Invalid Cluster Configuration",
			fmt.Sprintf("Invalid update parameters: %s", err.Error()),
		)
	default:
		diags.AddError(
			"Unable to Update Cluster",
			fmt.Sprintf("Unable to update cluster: %s", err.Error()),
		)
	}
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

	deleteReq := organizationv1.DeleteClusterRequest_builder{
		ClusterId: state.ID.ValueString(),
	}.Build()

	_, err := r.client.ClusterService.DeleteCluster(ctx, deleteReq)
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound, connect.CodePermissionDenied:
			// Once a deletion is requested, for example by a destroy that timed
			// out, the API answers like this for the cluster. Only a cluster
			// that is listed and not being deleted is a real refusal.
			listed, listErr := r.listedCluster(ctx, state.ID.ValueString())
			if listErr != nil {
				resp.Diagnostics.AddError("Unable to Delete Cluster", listErr.Error())
				return
			}
			if listed != nil && listed.GetStatus() != organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING {
				resp.Diagnostics.AddError("Unable to Delete Cluster", fmt.Sprintf("Unable to delete cluster %q: %s", state.Name.ValueString(), err.Error()))
				return
			}
			tflog.Info(ctx, "Cluster deletion already requested", map[string]any{
				"id": state.ID.ValueString(),
			})
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

	deleteTimeout, diags := state.Timeouts.Delete(ctx, defaultClusterDeleteTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	waitCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	if err := waitForClusterDeleted(waitCtx, r.client, state.ID.ValueString()); err != nil {
		detail := fmt.Sprintf("Cluster %q: %s.", state.Name.ValueString(), err)
		if waitInconclusive(err) {
			detail += " Fundament keeps deleting it; run the destroy again to wait for it."
		}
		resp.Diagnostics.AddError("Cluster Deletion Did Not Complete", detail)
		return
	}

	tflog.Info(ctx, "Deleted cluster", map[string]any{
		"id": state.ID.ValueString(),
	})
}

// ImportState imports an existing cluster into Terraform state.
func (r *ClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
