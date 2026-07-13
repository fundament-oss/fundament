package gardener

import (
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func workersOf(bounds ...[2]int32) []gardencorev1beta1.Worker {
	workers := make([]gardencorev1beta1.Worker, len(bounds))
	for i, b := range bounds {
		workers[i] = gardencorev1beta1.Worker{Minimum: b[0], Maximum: b[1]}
	}
	return workers
}

func poolsOf(maxima ...int32) []NodePool {
	pools := make([]NodePool, len(maxima))
	for i, m := range maxima {
		pools[i] = NodePool{AutoscaleMax: m}
	}
	return pools
}

func TestClampWorkerMaxima(t *testing.T) {
	tests := []struct {
		name   string
		in     [][2]int32
		cap    *int32
		expect [][2]int32
	}{
		{
			name:   "nil cap is a no-op",
			in:     [][2]int32{{1, 10}, {5, 20}},
			cap:    nil,
			expect: [][2]int32{{1, 10}, {5, 20}},
		},
		{
			name:   "maximum above the cap is clamped",
			in:     [][2]int32{{1, 10}},
			cap:    ptr.To[int32](5),
			expect: [][2]int32{{1, 5}},
		},
		{
			name:   "minimum above the clamped maximum is lowered",
			in:     [][2]int32{{8, 10}},
			cap:    ptr.To[int32](5),
			expect: [][2]int32{{5, 5}},
		},
		{
			name:   "worker within the cap is unchanged",
			in:     [][2]int32{{1, 4}},
			cap:    ptr.To[int32](5),
			expect: [][2]int32{{1, 4}},
		},
		{
			name:   "worker at the cap is unchanged",
			in:     [][2]int32{{1, 5}},
			cap:    ptr.To[int32](5),
			expect: [][2]int32{{1, 5}},
		},
		{
			name:   "mixed set clamps only the offenders",
			in:     [][2]int32{{1, 3}, {2, 8}, {7, 9}},
			cap:    ptr.To[int32](5),
			expect: [][2]int32{{1, 3}, {2, 5}, {5, 5}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workers := workersOf(tt.in...)
			clampWorkerMaxima(workers, tt.cap)
			require.Equal(t, workersOf(tt.expect...), workers)
		})
	}
}

func TestValidateNodeLimits(t *testing.T) {
	tests := []struct {
		name    string
		pools   []NodePool
		limits  NodeLimits
		wantErr string
	}{
		{
			name:   "no limits set passes",
			pools:  poolsOf(10, 10),
			limits: NodeLimits{},
		},
		{
			name:    "pool count over the cap fails",
			pools:   poolsOf(1, 1, 1),
			limits:  NodeLimits{MaxNodePoolsPerCluster: ptr.To[int32](2)},
			wantErr: "max_node_pools_per_cluster is 2 but the cluster defines 3 worker pools",
		},
		{
			name:   "pool count at the cap passes",
			pools:  poolsOf(1, 1),
			limits: NodeLimits{MaxNodePoolsPerCluster: ptr.To[int32](2)},
		},
		{
			name:    "total maximum over the cap fails",
			pools:   poolsOf(6, 5),
			limits:  NodeLimits{MaxNodesPerCluster: ptr.To[int32](10)},
			wantErr: "max_nodes_per_cluster is 10 but the worker pool maxima sum to 11",
		},
		{
			name:   "total maximum at the cap passes",
			pools:  poolsOf(5, 5),
			limits: NodeLimits{MaxNodesPerCluster: ptr.To[int32](10)},
		},
		{
			name:    "pool count violation reported before node total",
			pools:   poolsOf(6, 6, 6),
			limits:  NodeLimits{MaxNodePoolsPerCluster: ptr.To[int32](2), MaxNodesPerCluster: ptr.To[int32](10)},
			wantErr: "max_node_pools_per_cluster",
		},
		{
			// The per-pool clamp applies before the aggregate check: a pool set
			// whose raw maxima exceed the cluster cap passes once the per-pool
			// cap brings the total down.
			name:  "per-pool cap brings the total under the cluster cap",
			pools: poolsOf(20, 20),
			limits: NodeLimits{
				MaxNodesPerNodePool: ptr.To[int32](5),
				MaxNodesPerCluster:  ptr.To[int32](10),
			},
		},
		{
			name:   "no pools counts as one default worker",
			pools:  nil,
			limits: NodeLimits{MaxNodePoolsPerCluster: ptr.To[int32](1), MaxNodesPerCluster: ptr.To(defaultWorkerMaximum)},
		},
		{
			name:    "no pools default worker over the cluster cap fails",
			pools:   nil,
			limits:  NodeLimits{MaxNodesPerCluster: ptr.To[int32](2)},
			wantErr: "max_nodes_per_cluster is 2 but the worker pool maxima sum to 3",
		},
		{
			name:   "no pools default worker clamped under the cluster cap passes",
			pools:  nil,
			limits: NodeLimits{MaxNodesPerNodePool: ptr.To[int32](2), MaxNodesPerCluster: ptr.To[int32](2)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNodeLimits(tt.pools, tt.limits)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestEffectiveWorkerCounts(t *testing.T) {
	poolCount, totalMaximum := effectiveWorkerCounts(nil, nil)
	require.Equal(t, 1, poolCount)
	require.Equal(t, defaultWorkerMaximum, totalMaximum)

	poolCount, totalMaximum = effectiveWorkerCounts(poolsOf(4, 8), ptr.To[int32](5))
	require.Equal(t, 2, poolCount)
	require.Equal(t, int32(9), totalMaximum)
}

func TestClampedNodePoolMaximum(t *testing.T) {
	require.Equal(t, int32(10), clampedNodePoolMaximum(10, nil))
	require.Equal(t, int32(5), clampedNodePoolMaximum(10, ptr.To[int32](5)))
	require.Equal(t, int32(3), clampedNodePoolMaximum(3, ptr.To[int32](5)))
}
