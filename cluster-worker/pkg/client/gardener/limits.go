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
// fail the apply instead, see validateAggregateNodeLimits.
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

// validateAggregateNodeLimits checks the caps that have no per-worker Gardener
// field against a built (already clamped) worker set. A violation fails the
// apply rather than silently shrinking or dropping pools.
func validateAggregateNodeLimits(workers []gardencorev1beta1.Worker, limits NodeLimits) error {
	var totalMaximum int32
	for i := range workers {
		totalMaximum += workers[i].Maximum
	}
	return validateAggregateNodeLimitCounts(len(workers), totalMaximum, limits)
}

// validateAggregateNodeLimitCounts is the gardener-type-free core of
// validateAggregateNodeLimits, shared with the mock client.
func validateAggregateNodeLimitCounts(poolCount int, totalMaximum int32, limits NodeLimits) error {
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

// clampedNodePoolMaximum returns a node pool's effective autoscaler maximum
// under the per-pool cap. Mirror of clampWorkerMaxima for callers that reason
// about NodePool data instead of built workers (the mock client).
func clampedNodePoolMaximum(autoscaleMax int32, maxNodesPerNodePool *int32) int32 {
	if maxNodesPerNodePool != nil && autoscaleMax > *maxNodesPerNodePool {
		return *maxNodesPerNodePool
	}
	return autoscaleMax
}
