package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderCephObjectStore(t *testing.T) {
	t.Parallel()
	u := RenderCephObjectStore("rook-ceph", "cephobj-main", 3, "host", 2)
	assert.Equal(t, "ceph.rook.io/v1", u.GetAPIVersion())
	assert.Equal(t, "CephObjectStore", u.GetKind())
	assert.Equal(t, "cephobj-main", u.GetName())
	assert.Equal(t, "rook-ceph", u.GetNamespace())

	for _, pool := range []string{"metadataPool", "dataPool"} {
		size, found, err := unstructured.NestedInt64(u.Object, "spec", pool, "replicated", "size")
		require.NoError(t, err)
		require.True(t, found, pool)
		assert.Equal(t, int64(3), size, pool)
		fd, _, _ := unstructured.NestedString(u.Object, "spec", pool, "failureDomain")
		assert.Equal(t, "host", fd, pool)
	}

	preserve, found, err := unstructured.NestedBool(u.Object, "spec", "preservePoolsOnDelete")
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, preserve, "deleting the CR must never destroy data")

	instances, _, _ := unstructured.NestedInt64(u.Object, "spec", "gateway", "instances")
	assert.Equal(t, int64(2), instances)
	port, _, _ := unstructured.NestedInt64(u.Object, "spec", "gateway", "port")
	assert.Equal(t, int64(80), port)
}

// Ceph rejects size-1 pools unless the safety check is waived, same as RBD.
func TestRenderCephObjectStoreSingleReplica(t *testing.T) {
	t.Parallel()
	u := RenderCephObjectStore("rook-ceph", "cephobj-main", 1, "osd", 1)

	for _, pool := range []string{"metadataPool", "dataPool"} {
		safe, found, err := unstructured.NestedBool(u.Object, "spec", pool, "replicated", "requireSafeReplicaSize")
		require.NoError(t, err)
		require.True(t, found, pool)
		assert.False(t, safe, pool)
	}
}
