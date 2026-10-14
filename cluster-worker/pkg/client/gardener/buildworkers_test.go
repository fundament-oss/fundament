package gardener

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/kubename"
)

// A cluster without node pools still gets one worker group, bounded 1 to 3,
// on the provider's default machine type.
func TestBuildWorkers_NoPoolsGetsTheDefaultWorker(t *testing.T) {
	r := testClient(NewProviderConfig())

	workers := r.buildWorkers(testCluster())

	require.Len(t, workers, 1)
	require.Equal(t, "default", workers[0].Name)
	require.Equal(t, r.provider.DefaultMachineType, workers[0].Machine.Type)
	require.Equal(t, defaultWorkerMinimum, workers[0].Minimum)
	require.Equal(t, defaultWorkerMaximum, workers[0].Maximum)
	require.Equal(t, int32(1), workers[0].MaxSurge.IntVal)
	require.Equal(t, int32(0), workers[0].MaxUnavailable.IntVal)
}

// Explicit pools pass through as they are: there is no organization cap to
// clamp a maximum to any more, so a large pool stays large. A non-local
// provider keeps the pool's machine type; the local one substitutes its own.
func TestBuildWorkers_PoolsPassThroughUnchanged(t *testing.T) {
	provider := NewProviderConfig()
	provider.Type = "metal"
	r := testClient(provider)
	cluster := testCluster()
	cluster.NodePools = []NodePool{
		{Name: "small", MachineType: "local-small", AutoscaleMin: 0, AutoscaleMax: 2},
		{Name: "large", MachineType: "local-medium", AutoscaleMin: 5, AutoscaleMax: 250},
	}

	workers := r.buildWorkers(cluster)

	require.Len(t, workers, 2)
	require.Equal(t, kubename.GenerateWorkerName("small"), workers[0].Name)
	require.Equal(t, "local-small", workers[0].Machine.Type)
	require.Equal(t, int32(0), workers[0].Minimum)
	require.Equal(t, int32(2), workers[0].Maximum)
	require.Equal(t, kubename.GenerateWorkerName("large"), workers[1].Name)
	require.Equal(t, "local-medium", workers[1].Machine.Type)
	require.Equal(t, int32(5), workers[1].Minimum)
	require.Equal(t, int32(250), workers[1].Maximum)

	// Each worker carries its own surge settings and image version, not a
	// pointer shared across the loop.
	require.NotSame(t, workers[0].MaxSurge, workers[1].MaxSurge)
	require.NotSame(t, workers[0].Machine.Image.Version, workers[1].Machine.Image.Version)
	require.Equal(t, r.provider.MachineImageVersion, *workers[1].Machine.Image.Version)
}
