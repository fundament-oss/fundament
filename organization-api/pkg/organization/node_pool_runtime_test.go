package organization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func poolLabel(node, pool string) prom.Sample {
	return prom.Sample{
		Labels: map[string]string{"node": node, nodePoolNodeLabel: pool},
		Value:  1,
	}
}

func readySample(node string, ready bool) prom.Sample {
	value := 0.0
	if ready {
		value = 1
	}
	return prom.Sample{Labels: map[string]string{"node": node, "condition": "Ready"}, Value: value}
}

func infoSample(node, version string) prom.Sample {
	return prom.Sample{Labels: map[string]string{"node": node, "kubelet_version": version}, Value: 1}
}

func TestNodePoolRuntimesFromSamples_CountsPerPool(t *testing.T) {
	runtimes := nodePoolRuntimesFromSamples(
		[]prom.Sample{
			poolLabel("node-a", "workers"),
			poolLabel("node-b", "workers"),
			poolLabel("node-c", "spot"),
		},
		[]prom.Sample{
			readySample("node-a", true),
			readySample("node-b", true),
			readySample("node-c", true),
		},
		[]prom.Sample{
			infoSample("node-a", "v1.31.4"),
			infoSample("node-b", "v1.31.4"),
			infoSample("node-c", "v1.31.4"),
		},
	)

	assert.Equal(t, int32(2), runtimes["workers"].nodes)
	assert.Equal(t, int32(2), runtimes["workers"].ready)
	assert.Equal(t, "v1.31.4", runtimes["workers"].version)
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY, runtimes["workers"].status())

	assert.Equal(t, int32(1), runtimes["spot"].nodes)
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY, runtimes["spot"].status())
}

func TestNodePoolRuntimesFromSamples_Status(t *testing.T) {
	tests := []struct {
		name  string
		ready []bool
		want  organizationv1.NodePoolStatus
	}{
		{"all ready", []bool{true, true}, organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY},
		{"some ready", []bool{true, false}, organizationv1.NodePoolStatus_NODE_POOL_STATUS_DEGRADED},
		{"none ready", []bool{false, false}, organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNHEALTHY},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var membership, ready []prom.Sample
			for i, isReady := range tc.ready {
				node := string(rune('a' + i))
				membership = append(membership, poolLabel(node, "workers"))
				ready = append(ready, readySample(node, isReady))
			}

			runtimes := nodePoolRuntimesFromSamples(membership, ready, nil)

			assert.Equal(t, tc.want, runtimes["workers"].status())
		})
	}
}

// A node Prometheus never mentions is a node that is not up: it has no Ready
// series at all, and the pool must not read as healthy because of it.
func TestNodePoolRuntimesFromSamples_MissingReadySeriesIsNotReady(t *testing.T) {
	runtimes := nodePoolRuntimesFromSamples(
		[]prom.Sample{poolLabel("node-a", "workers"), poolLabel("node-b", "workers")},
		[]prom.Sample{readySample("node-a", true)},
		nil,
	)

	assert.Equal(t, int32(2), runtimes["workers"].nodes)
	assert.Equal(t, int32(1), runtimes["workers"].ready)
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_DEGRADED, runtimes["workers"].status())
}

// Mid-upgrade a pool runs two kubelet versions; naming one of them would be a
// guess, so the pool reports none and the console hides the row.
func TestNodePoolRuntimesFromSamples_MixedVersions(t *testing.T) {
	runtimes := nodePoolRuntimesFromSamples(
		[]prom.Sample{poolLabel("node-a", "workers"), poolLabel("node-b", "workers")},
		[]prom.Sample{readySample("node-a", true), readySample("node-b", true)},
		[]prom.Sample{infoSample("node-a", "v1.31.4"), infoSample("node-b", "v1.32.1")},
	)

	assert.Empty(t, runtimes["workers"].version)
}

// Nodes without the Gardener pool label belong to no pool we know of.
func TestNodePoolRuntimesFromSamples_UnlabelledNodesIgnored(t *testing.T) {
	runtimes := nodePoolRuntimesFromSamples(
		[]prom.Sample{poolLabel("node-a", "workers"), {Labels: map[string]string{"node": "node-b"}, Value: 1}},
		[]prom.Sample{readySample("node-a", true), readySample("node-b", true)},
		nil,
	)

	assert.Len(t, runtimes, 1)
	assert.Equal(t, int32(1), runtimes["workers"].nodes)
}

// No metrics backend at all: every pool keeps the zero value, which the console
// reads as an unknown status rather than an empty or broken card.
func TestNodePoolRuntime_ZeroValueIsUnknown(t *testing.T) {
	var runtime nodePoolRuntime

	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, runtime.status())
	assert.Equal(t, int32(0), runtime.nodes)
	assert.Empty(t, runtime.version)
}
