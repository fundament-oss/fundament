package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderCephBlockPool(t *testing.T) {
	u := RenderCephBlockPool("rook-ceph", "pool-a", 3, "host")
	assert.Equal(t, "ceph.rook.io/v1", u.GetAPIVersion())
	assert.Equal(t, "CephBlockPool", u.GetKind())
	assert.Equal(t, "pool-a", u.GetName())
	assert.Equal(t, "rook-ceph", u.GetNamespace())

	size, found, err := unstructured.NestedInt64(u.Object, "spec", "replicated", "size")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, int64(3), size)

	fd, _, _ := unstructured.NestedString(u.Object, "spec", "failureDomain")
	assert.Equal(t, "host", fd)

	safe, found, err := unstructured.NestedBool(u.Object, "spec", "replicated", "requireSafeReplicaSize")
	require.NoError(t, err)
	require.True(t, found, "always emitted; see replicatedPoolSpec")
	assert.True(t, safe)
}

// The key must be emitted at every size, or a size-1 waiver stays latched
// after scaling up; replicatedPoolSpec documents why.
func TestReplicatedPoolSpecAlwaysPinsSafeReplicaSize(t *testing.T) {
	t.Parallel()
	for replicas, want := range map[int]bool{1: false, 2: true, 3: true} {
		safe := replicatedPoolSpec(replicas, "host")["replicated"].(map[string]any)["requireSafeReplicaSize"]
		assert.Equal(t, want, safe, "replicas=%d", replicas)
	}
}

// Ceph rejects a size-1 pool unless the safe-replica check is waived, which is
// what an absent spec.replicas yields on one node.
func TestRenderCephBlockPoolSingleReplica(t *testing.T) {
	u := RenderCephBlockPool("rook-ceph", "pool-a", 1, "osd")

	safe, found, err := unstructured.NestedBool(u.Object, "spec", "replicated", "requireSafeReplicaSize")
	require.NoError(t, err)
	require.True(t, found)
	assert.False(t, safe)
}
