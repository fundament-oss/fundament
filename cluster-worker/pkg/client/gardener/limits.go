package gardener

import (
	"fmt"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
)

// Default worker group autoscaler bounds used when a cluster has no node pools.
const (
	defaultWorkerMinimum int32 = 1
	defaultWorkerMaximum int32 = 3
)

// clampWorkerMaxima enforces the per-pool node cap on a built worker set, in
// place: each worker's Maximum is lowered to the cap, and its Minimum is
// lowered to the clamped Maximum when it would otherwise exceed it (preserving
// Minimum <= Maximum). A nil cap is a no-op. Clamping is the intended cap
// semantic ("applies to autoscaler max"), so it is silent; the aggregate caps
// fail the apply instead, see validateNodeLimits.
func clampWorkerMaxima(workers []gardencorev1beta1.Worker, maxNodesPerNodePool *int32) {
	if maxNodesPerNodePool == nil {
		return
	}
	for i := range workers {
		if workers[i].Maximum > *maxNodesPerNodePool {
			workers[i].Maximum = *maxNodesPerNodePool
		}
		if workers[i].Minimum > workers[i].Maximum {
			workers[i].Minimum = workers[i].Maximum
		}
	}
}

// clampedNodePoolMaximum returns a node pool's effective autoscaler maximum
// under the per-pool cap. Mirror of clampWorkerMaxima for callers that reason
// about NodePool data instead of built workers.
func clampedNodePoolMaximum(autoscaleMax int32, maxNodesPerNodePool *int32) int32 {
	if maxNodesPerNodePool != nil && autoscaleMax > *maxNodesPerNodePool {
		return *maxNodesPerNodePool
	}
	return autoscaleMax
}

// effectiveWorkerCounts returns the worker pool count and the sum of effective
// (per-pool-cap clamped) autoscaler maxima that buildWorkers produces for the
// given pools, including the "no pools ⇒ one default worker" rule.
func effectiveWorkerCounts(pools []NodePool, maxNodesPerNodePool *int32) (poolCount int, totalMaximum int32) {
	if len(pools) == 0 {
		return 1, clampedNodePoolMaximum(defaultWorkerMaximum, maxNodesPerNodePool)
	}
	for _, np := range pools {
		totalMaximum += clampedNodePoolMaximum(np.AutoscaleMax, maxNodesPerNodePool)
	}
	return len(pools), totalMaximum
}

// validateNodeLimits checks the caps that have no per-worker Gardener field
// against a cluster's effective worker set. A violation fails the apply rather
// than silently shrinking or dropping pools. Shared by the real and mock
// clients so both enforce the same rules.
func validateNodeLimits(pools []NodePool, limits NodeLimits) error {
	poolCount, totalMaximum := effectiveWorkerCounts(pools, limits.MaxNodesPerNodePool)
	if limits.MaxNodePoolsPerCluster != nil && poolCount > int(*limits.MaxNodePoolsPerCluster) {
		return fmt.Errorf("organization node limit exceeded: max_node_pools_per_cluster is %d but the cluster defines %d worker pools",
			*limits.MaxNodePoolsPerCluster, poolCount)
	}
	if limits.MaxNodesPerCluster != nil && totalMaximum > *limits.MaxNodesPerCluster {
		return fmt.Errorf("organization node limit exceeded: max_nodes_per_cluster is %d but the worker pool maxima sum to %d",
			*limits.MaxNodesPerCluster, totalMaximum)
	}
	return nil
}
