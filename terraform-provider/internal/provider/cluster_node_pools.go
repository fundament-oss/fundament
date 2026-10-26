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
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
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
		Description: "A node pool of the cluster, at least one: a cluster runs only on its node pools and starts once it has one. " +
			"The blocks are the cluster's complete set of node pools: " +
			"pools added elsewhere, such as in the console, are removed on the next apply.",
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
		Validators: []validator.List{listvalidator.SizeAtLeast(1), nodePoolsValidator{}},
	}
}

// nodePoolsValidator rejects two node_pool blocks with the same name, and a
// pool whose maximum is below its minimum.
type nodePoolsValidator struct{}

func (v nodePoolsValidator) Description(_ context.Context) string {
	return "node pool names must be unique within the cluster, autoscale_max at least autoscale_min, and at least one pool with autoscale_max above 0"
}

func (v nodePoolsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v nodePoolsValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) { //nolint:gocritic // signature set by validator.List
	// A list validator such as SizeAtLeast skips a null list, so refuse a
	// missing node_pool here: the cluster would never get a Shoot.
	if req.ConfigValue.IsNull() {
		resp.Diagnostics.AddAttributeError(req.Path, "Missing Node Pool", "At least one node_pool block is required.")
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}

	var pools []ClusterNodePoolModel
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &pools, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A cluster needs a pool that can run nodes: the API refuses to leave a
	// cluster with only empty pools, and Gardener refuses such a Shoot.
	active := false
	for _, pool := range pools {
		if pool.AutoscaleMax.IsNull() || pool.AutoscaleMax.IsUnknown() || pool.AutoscaleMax.ValueInt64() > 0 {
			active = true
		}
	}
	if len(pools) > 0 && !active {
		resp.Diagnostics.AddAttributeError(req.Path, "No Node Pool Can Run Nodes",
			"At least one node_pool needs an autoscale_max above 0.")
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

type existingNodePool struct {
	ID   string
	Pool ClusterNodePoolModel
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

// setNodePools makes planned the cluster's complete set of node pools in one
// API call. The API applies it at once or not at all: the cluster never passes
// through zero pools or an intermediate set, and a pool replaced at full quota
// fits because the old one goes in the same step. Pools are matched by name, so
// one whose machine type changes is replaced under its name.
func setNodePools(ctx context.Context, client *FundamentClient, clusterID string, planned []ClusterNodePoolModel) diag.Diagnostics {
	var diags diag.Diagnostics

	specs := make([]*organizationv1.NodePoolSpec, 0, len(planned))
	for _, pool := range planned {
		specs = append(specs, organizationv1.NodePoolSpec_builder{
			Name:         pool.Name.ValueString(),
			MachineType:  pool.MachineType.ValueString(),
			AutoscaleMin: int32(pool.AutoscaleMin.ValueInt64()), //nolint:gosec // the schema bounds it to int32
			AutoscaleMax: int32(pool.AutoscaleMax.ValueInt64()), //nolint:gosec // the schema bounds it to int32
		}.Build())
	}

	tflog.Debug(ctx, "Setting node pools", map[string]any{"cluster_id": clusterID, "count": len(specs)})
	// The whole set is idempotent, so a call whose answer was lost is simply
	// made again: without this, a dropped connection right after CreateCluster
	// would taint the cluster and the next apply would recreate it.
	err := retryTransient(ctx, defaultClock, func(ctx context.Context) error {
		_, err := client.ClusterService.SetNodePools(ctx, organizationv1.SetNodePoolsRequest_builder{
			ClusterId: clusterID,
			NodePools: specs,
		}.Build())
		return err
	})
	if err != nil {
		diags.AddError("Unable to Set Node Pools", err.Error())
	}
	return diags
}

// retryTransient runs call again, with backoff, while it fails for a reason
// that says nothing about the request: the server was unreachable or did not
// answer in time. A refusal is returned as is.
func retryTransient(ctx context.Context, clk clock, call func(ctx context.Context) error) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, idempotencyTotalBudget)
	defer cancel()

	backoff := idempotencyInitialBackoff
	for {
		err := call(deadlineCtx)
		if err == nil || !transient(err) {
			return err
		}
		if sleepErr := clk.Sleep(deadlineCtx, backoff); sleepErr != nil {
			return err
		}
		backoff = min(backoff*2, idempotencyMaxBackoff)
	}
}

// transient reports whether an error is worth another try: the server could
// not be reached, or the call ran out of time before it answered.
func transient(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded:
		return true
	default:
		return false
	}
}

// sameNodePools reports whether two sets of pools are the same, in any order:
// an apply whose planned pools are what the cluster runs needs no node pool call.
func sameNodePools(a, b []ClusterNodePoolModel) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(pool ClusterNodePoolModel) string {
		return fmt.Sprintf("%s|%s|%d|%d", pool.Name.ValueString(), pool.MachineType.ValueString(),
			pool.AutoscaleMin.ValueInt64(), pool.AutoscaleMax.ValueInt64())
	}
	counts := make(map[string]int, len(a))
	for _, pool := range a {
		counts[key(pool)]++
	}
	for _, pool := range b {
		counts[key(pool)]--
		if counts[key(pool)] < 0 {
			return false
		}
	}
	return true
}

// newOrRetyped returns the planned pools that are not already running under
// the same name and machine type: the ones whose machine type the region has
// to offer. A pool that keeps its machine type may be a legacy one the catalog
// no longer lists; it keeps running and must not block other changes.
func newOrRetyped(current, planned []ClusterNodePoolModel) []ClusterNodePoolModel {
	running := make(map[string]string, len(current))
	for _, pool := range current {
		running[pool.Name.ValueString()] = pool.MachineType.ValueString()
	}
	var out []ClusterNodePoolModel
	for _, pool := range planned {
		if machineType, ok := running[pool.Name.ValueString()]; !ok || machineType != pool.MachineType.ValueString() {
			out = append(out, pool)
		}
	}
	return out
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
