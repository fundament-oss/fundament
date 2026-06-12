package gardener

import (
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/require"
)

func int32ptr(v int32) *int32 { return &v }

func workersOf(bounds ...[2]int32) []gardencorev1beta1.Worker {
	workers := make([]gardencorev1beta1.Worker, len(bounds))
	for i, b := range bounds {
		workers[i] = gardencorev1beta1.Worker{Minimum: b[0], Maximum: b[1]}
	}
	return workers
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
			cap:    int32ptr(5),
			expect: [][2]int32{{1, 5}},
		},
		{
			name:   "minimum above the clamped maximum is lowered",
			in:     [][2]int32{{8, 10}},
			cap:    int32ptr(5),
			expect: [][2]int32{{5, 5}},
		},
		{
			name:   "worker within the cap is unchanged",
			in:     [][2]int32{{1, 4}},
			cap:    int32ptr(5),
			expect: [][2]int32{{1, 4}},
		},
		{
			name:   "worker at the cap is unchanged",
			in:     [][2]int32{{1, 5}},
			cap:    int32ptr(5),
			expect: [][2]int32{{1, 5}},
		},
		{
			name:   "mixed set clamps only the offenders",
			in:     [][2]int32{{1, 3}, {2, 8}, {7, 9}},
			cap:    int32ptr(5),
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

func TestValidateAggregateNodeLimits(t *testing.T) {
	tests := []struct {
		name    string
		workers [][2]int32
		limits  NodeLimits
		wantErr string
	}{
		{
			name:    "no limits set passes",
			workers: [][2]int32{{1, 10}, {1, 10}},
			limits:  NodeLimits{},
		},
		{
			name:    "pool count over the cap fails",
			workers: [][2]int32{{1, 1}, {1, 1}, {1, 1}},
			limits:  NodeLimits{MaxNodePoolsPerCluster: int32ptr(2)},
			wantErr: "max_node_pools_per_cluster is 2 but the cluster defines 3 worker pools",
		},
		{
			name:    "pool count at the cap passes",
			workers: [][2]int32{{1, 1}, {1, 1}},
			limits:  NodeLimits{MaxNodePoolsPerCluster: int32ptr(2)},
		},
		{
			name:    "total maximum over the cap fails",
			workers: [][2]int32{{1, 6}, {1, 5}},
			limits:  NodeLimits{MaxNodesPerCluster: int32ptr(10)},
			wantErr: "max_nodes_per_cluster is 10 but the worker pool maxima sum to 11",
		},
		{
			name:    "total maximum at the cap passes",
			workers: [][2]int32{{1, 5}, {1, 5}},
			limits:  NodeLimits{MaxNodesPerCluster: int32ptr(10)},
		},
		{
			name:    "pool count violation reported before node total",
			workers: [][2]int32{{1, 6}, {1, 6}, {1, 6}},
			limits:  NodeLimits{MaxNodePoolsPerCluster: int32ptr(2), MaxNodesPerCluster: int32ptr(10)},
			wantErr: "max_node_pools_per_cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAggregateNodeLimits(workersOf(tt.workers...), tt.limits)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// The clamp applies before the aggregate check: a pool set whose raw maxima
// exceed the cluster cap passes once the per-pool cap brings the total down.
func TestClampThenValidateInteraction(t *testing.T) {
	workers := workersOf([2]int32{1, 20}, [2]int32{1, 20})
	limits := NodeLimits{
		MaxNodesPerNodePool: int32ptr(5),
		MaxNodesPerCluster:  int32ptr(10),
	}
	clampWorkerMaxima(workers, limits.MaxNodesPerNodePool)
	require.NoError(t, validateAggregateNodeLimits(workers, limits))
}

func TestClampedNodePoolMaximum(t *testing.T) {
	require.Equal(t, int32(10), clampedNodePoolMaximum(10, nil))
	require.Equal(t, int32(5), clampedNodePoolMaximum(10, int32ptr(5)))
	require.Equal(t, int32(3), clampedNodePoolMaximum(3, int32ptr(5)))
}
