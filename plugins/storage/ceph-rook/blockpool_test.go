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

	_, found, err = unstructured.NestedBool(u.Object, "spec", "replicated", "requireSafeReplicaSize")
	require.NoError(t, err)
	assert.False(t, found)
}

// Ceph rejects a size-1 pool unless the safe-replica check is waived, which is
// what `replication: auto` yields on one node.
func TestRenderCephBlockPoolSingleReplica(t *testing.T) {
	u := RenderCephBlockPool("rook-ceph", "pool-a", 1, "osd")

	safe, found, err := unstructured.NestedBool(u.Object, "spec", "replicated", "requireSafeReplicaSize")
	require.NoError(t, err)
	require.True(t, found)
	assert.False(t, safe)
}
