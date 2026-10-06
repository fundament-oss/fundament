package provider

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ClusterNodePoolModel is one node_pool block of fundament_cluster.
type ClusterNodePoolModel struct {
	Name         types.String `tfsdk:"name"`
	MachineType  types.String `tfsdk:"machine_type"`
	AutoscaleMin types.Int64  `tfsdk:"autoscale_min"`
	AutoscaleMax types.Int64  `tfsdk:"autoscale_max"`
}

// The console's rule; Gardener derives the worker name from it.
var nodePoolNamePattern = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)

func clusterNodePoolBlock() schema.ListNestedBlock {
	return schema.ListNestedBlock{
		Description: "A node pool of the cluster. The blocks are the cluster's complete set of node pools: " +
			"pools added elsewhere, such as in the console, are removed on the next apply. " +
			"Without any node_pool block, Gardener runs a default worker pool that fundament does not list.",
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Description: "The name of the node pool, unique within the cluster: lowercase letters, digits and hyphens, starting with a letter.",
					Required:    true,
					Validators: []validator.String{
						stringvalidator.LengthBetween(1, 63),
						stringvalidator.RegexMatches(nodePoolNamePattern, "must consist of lowercase letters, digits and hyphens, start with a letter and not end with a hyphen"),
					},
				},
				"machine_type": schema.StringAttribute{
					Description: "The machine type of the nodes, one the cluster's region offers. Changing it replaces the node pool and its nodes.",
					Required:    true,
				},
				"autoscale_min": schema.Int64Attribute{
					Description: "The minimum number of nodes.",
					Required:    true,
					Validators:  []validator.Int64{int64validator.Between(0, math.MaxInt32)},
				},
				"autoscale_max": schema.Int64Attribute{
					Description: "The maximum number of nodes. Must be at least autoscale_min.",
					Required:    true,
					Validators:  []validator.Int64{int64validator.Between(0, math.MaxInt32)},
				},
			},
		},
		Validators: []validator.List{nodePoolsValidator{}},
	}
}

// nodePoolsValidator rejects two node_pool blocks with the same name, and a
// pool whose maximum is below its minimum.
type nodePoolsValidator struct{}

func (v nodePoolsValidator) Description(_ context.Context) string {
	return "node pool names must be unique within the cluster, and autoscale_max at least autoscale_min"
}

func (v nodePoolsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v nodePoolsValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) { //nolint:gocritic // signature set by validator.List
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var pools []ClusterNodePoolModel
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &pools, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	seen := map[string]bool{}
	for i, pool := range pools {
		if !pool.AutoscaleMin.IsNull() && !pool.AutoscaleMin.IsUnknown() &&
			!pool.AutoscaleMax.IsNull() && !pool.AutoscaleMax.IsUnknown() &&
			pool.AutoscaleMax.ValueInt64() < pool.AutoscaleMin.ValueInt64() {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtListIndex(i).AtName("autoscale_max"),
				"Invalid Node Pool Size",
				fmt.Sprintf("autoscale_max (%d) must be at least autoscale_min (%d).", pool.AutoscaleMax.ValueInt64(), pool.AutoscaleMin.ValueInt64()),
			)
		}

		if pool.Name.IsNull() || pool.Name.IsUnknown() {
			continue
		}
		name := pool.Name.ValueString()
		if seen[name] {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtListIndex(i).AtName("name"),
				"Duplicate Node Pool Name",
				fmt.Sprintf("Node pool name %q is used more than once.", name),
			)
		}
		seen[name] = true
	}
}

// nodePoolChanges are the API calls that turn the current node pools into the planned ones.
type nodePoolChanges struct {
	// Create and Delete hold a pool whose machine type changes, under the same name.
	Create []ClusterNodePoolModel
	Update []existingNodePool
	Delete []existingNodePool
}

type existingNodePool struct {
	ID   string
	Pool ClusterNodePoolModel
}

func planNodePoolChanges(current []existingNodePool, planned []ClusterNodePoolModel) nodePoolChanges {
	var changes nodePoolChanges

	byName := map[string]existingNodePool{}
	for _, pool := range current {
		byName[pool.Pool.Name.ValueString()] = pool
	}

	plannedNames := map[string]bool{}
	for _, pool := range planned {
		plannedNames[pool.Name.ValueString()] = true

		existing, ok := byName[pool.Name.ValueString()]
		switch {
		case !ok:
			changes.Create = append(changes.Create, pool)
		case existing.Pool.MachineType.ValueString() != pool.MachineType.ValueString():
			// The API cannot change a pool's machine type: replace the pool.
			changes.Delete = append(changes.Delete, existing)
			changes.Create = append(changes.Create, pool)
		case existing.Pool.AutoscaleMin.ValueInt64() != pool.AutoscaleMin.ValueInt64(),
			existing.Pool.AutoscaleMax.ValueInt64() != pool.AutoscaleMax.ValueInt64():
			changes.Update = append(changes.Update, existingNodePool{ID: existing.ID, Pool: pool})
		}
	}

	for _, pool := range current {
		if !plannedNames[pool.Pool.Name.ValueString()] {
			changes.Delete = append(changes.Delete, pool)
		}
	}

	return changes
}

// listNodePools reads the cluster's node pools from the API.
func listNodePools(ctx context.Context, client *FundamentClient, clusterID string) ([]existingNodePool, error) {
	resp, err := client.ClusterService.ListNodePools(ctx, organizationv1.ListNodePoolsRequest_builder{
		ClusterId: clusterID,
	}.Build())
	if err != nil {
		return nil, fmt.Errorf("list node pools: %w", err)
	}

	pools := make([]existingNodePool, 0, len(resp.GetNodePools()))
	for _, np := range resp.GetNodePools() {
		pools = append(pools, existingNodePool{
			ID: np.GetId(),
			Pool: ClusterNodePoolModel{
				Name:         types.StringValue(np.GetName()),
				MachineType:  types.StringValue(np.GetMachineType()),
				AutoscaleMin: types.Int64Value(int64(np.GetMinNodes())),
				AutoscaleMax: types.Int64Value(int64(np.GetMaxNodes())),
			},
		})
	}
	return pools, nil
}

// orderNodePools returns the pools in the order of the configuration, so that
// a refresh does not show a reordered list as a change. Pools the configuration
// does not have follow, sorted by name.
func orderNodePools(pools []existingNodePool, prior []ClusterNodePoolModel) []ClusterNodePoolModel {
	position := map[string]int{}
	for i, pool := range prior {
		position[pool.Name.ValueString()] = i
	}

	sorted := slices.Clone(pools)
	slices.SortStableFunc(sorted, func(a, b existingNodePool) int {
		posA, okA := position[a.Pool.Name.ValueString()]
		posB, okB := position[b.Pool.Name.ValueString()]
		switch {
		case okA && okB:
			return posA - posB
		case okA:
			return -1
		case okB:
			return 1
		default:
			return strings.Compare(a.Pool.Name.ValueString(), b.Pool.Name.ValueString())
		}
	})

	result := make([]ClusterNodePoolModel, 0, len(sorted))
	for _, pool := range sorted {
		result = append(result, pool.Pool)
	}
	return result
}

// applyNodePoolChanges makes the cluster's node pools match planned. New pools
// are created before old ones are removed, so the cluster keeps workers in between.
func applyNodePoolChanges(ctx context.Context, client *FundamentClient, clusterID, region string, planned []ClusterNodePoolModel) diag.Diagnostics {
	var diags diag.Diagnostics

	current, err := listNodePools(ctx, client, clusterID)
	if err != nil {
		diags.AddError("Unable to Read Node Pools", fmt.Sprintf("Cluster %q: %s", clusterID, err))
		return diags
	}

	changes := planNodePoolChanges(current, planned)

	// A pool whose machine type changes is deleted before its replacement is
	// created; check the new machine types first, so a refused create cannot
	// leave the cluster without the pool.
	diags.Append(checkMachineTypes(ctx, client, region, changes.Create)...)
	if diags.HasError() {
		return diags
	}

	// A pool whose machine type changes keeps its name, which must be free first.
	replaced := map[string]bool{}
	for _, pool := range changes.Create {
		replaced[pool.Name.ValueString()] = true
	}
	for i := range changes.Delete {
		if replaced[changes.Delete[i].Pool.Name.ValueString()] {
			diags.Append(deleteNodePool(ctx, client, &changes.Delete[i])...)
		}
	}
	if diags.HasError() {
		return diags
	}

	for i := range changes.Create {
		diags.Append(createNodePool(ctx, client, clusterID, &changes.Create[i])...)
		if diags.HasError() {
			return diags
		}
	}

	for _, pool := range changes.Update {
		tflog.Debug(ctx, "Updating node pool", map[string]any{"cluster_id": clusterID, "name": pool.Pool.Name.ValueString()})
		_, err := client.ClusterService.UpdateNodePool(ctx, organizationv1.UpdateNodePoolRequest_builder{
			NodePoolId:   pool.ID,
			AutoscaleMin: int32(pool.Pool.AutoscaleMin.ValueInt64()), //nolint:gosec // the schema bounds it to int32
			AutoscaleMax: int32(pool.Pool.AutoscaleMax.ValueInt64()), //nolint:gosec // the schema bounds it to int32
		}.Build())
		if err != nil {
			diags.AddError("Unable to Update Node Pool", fmt.Sprintf("Node pool %q: %s", pool.Pool.Name.ValueString(), err))
			return diags
		}
	}

	for i := range changes.Delete {
		if !replaced[changes.Delete[i].Pool.Name.ValueString()] {
			diags.Append(deleteNodePool(ctx, client, &changes.Delete[i])...)
			if diags.HasError() {
				return diags
			}
		}
	}

	return diags
}

// checkMachineTypes reports every pool whose machine type the region's catalog does not offer.
func checkMachineTypes(ctx context.Context, client *FundamentClient, region string, pools []ClusterNodePoolModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if len(pools) == 0 {
		return diags
	}

	resp, err := client.ClusterService.ListRegions(ctx, organizationv1.ListRegionsRequest_builder{}.Build())
	if err != nil {
		diags.AddError("Unable to Read the Region Catalog", err.Error())
		return diags
	}

	idx := slices.IndexFunc(resp.GetRegions(), func(r *organizationv1.Region) bool { return r.GetName() == region })
	if idx < 0 {
		diags.AddError("Unknown Region", fmt.Sprintf("Region %q is not in the catalog.", region))
		return diags
	}

	var offered []string
	for _, mt := range resp.GetRegions()[idx].GetMachineTypes() {
		offered = append(offered, mt.GetName())
	}
	for _, pool := range pools {
		if !slices.Contains(offered, pool.MachineType.ValueString()) {
			diags.AddError("Machine Type Not Offered",
				fmt.Sprintf("Node pool %q: region %q does not offer machine type %q; it offers %s.",
					pool.Name.ValueString(), region, pool.MachineType.ValueString(), strings.Join(offered, ", ")))
		}
	}
	return diags
}

func createNodePool(ctx context.Context, client *FundamentClient, clusterID string, pool *ClusterNodePoolModel) diag.Diagnostics {
	var diags diag.Diagnostics

	tflog.Debug(ctx, "Creating node pool", map[string]any{"cluster_id": clusterID, "name": pool.Name.ValueString()})
	_, err := createIdempotent(ctx, client.ClusterService.CreateNodePool, organizationv1.CreateNodePoolRequest_builder{
		ClusterId:    clusterID,
		Name:         pool.Name.ValueString(),
		MachineType:  pool.MachineType.ValueString(),
		AutoscaleMin: int32(pool.AutoscaleMin.ValueInt64()), //nolint:gosec // the schema bounds it to int32
		AutoscaleMax: int32(pool.AutoscaleMax.ValueInt64()), //nolint:gosec // the schema bounds it to int32
	}.Build())
	if err != nil {
		diags.AddError("Unable to Create Node Pool", fmt.Sprintf("Node pool %q: %s", pool.Name.ValueString(), err))
	}
	return diags
}

func deleteNodePool(ctx context.Context, client *FundamentClient, pool *existingNodePool) diag.Diagnostics {
	var diags diag.Diagnostics

	tflog.Debug(ctx, "Deleting node pool", map[string]any{"name": pool.Pool.Name.ValueString()})
	_, err := client.ClusterService.DeleteNodePool(ctx, organizationv1.DeleteNodePoolRequest_builder{
		NodePoolId: pool.ID,
	}.Build())
	if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
		diags.AddError("Unable to Delete Node Pool", fmt.Sprintf("Node pool %q: %s", pool.Pool.Name.ValueString(), err))
	}
	return diags
}
