package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderCephFilesystem(t *testing.T) {
	t.Parallel()
	u := RenderCephFilesystem("rook-ceph", "cephfs-shared", 3, "host", 2)
	assert.Equal(t, "ceph.rook.io/v1", u.GetAPIVersion())
	assert.Equal(t, "CephFilesystem", u.GetKind())
	assert.Equal(t, "cephfs-shared", u.GetName())
	assert.Equal(t, "rook-ceph", u.GetNamespace())

	size, found, err := unstructured.NestedInt64(u.Object, "spec", "metadataPool", "replicated", "size")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, int64(3), size)
	fd, _, _ := unstructured.NestedString(u.Object, "spec", "metadataPool", "failureDomain")
	assert.Equal(t, "host", fd)

	pools, found, err := unstructured.NestedSlice(u.Object, "spec", "dataPools")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, pools, 1)
	data := pools[0].(map[string]any)
	assert.Equal(t, "data0", data["name"], "Rook names the pool <fsName>-<name>; the StorageClass depends on it")
	assert.Equal(t, int64(3), data["replicated"].(map[string]any)["size"])
	assert.Equal(t, "host", data["failureDomain"])

	preserve, found, err := unstructured.NestedBool(u.Object, "spec", "preserveFilesystemOnDelete")
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, preserve, "deleting the CR must never destroy data")

	active, _, _ := unstructured.NestedInt64(u.Object, "spec", "metadataServer", "activeCount")
	assert.Equal(t, int64(2), active)
	standby, _, _ := unstructured.NestedBool(u.Object, "spec", "metadataServer", "activeStandby")
	assert.True(t, standby)
}

// Ceph rejects size-1 pools unless the safety check is waived, same as RBD.
func TestRenderCephFilesystemSingleReplica(t *testing.T) {
	t.Parallel()
	u := RenderCephFilesystem("rook-ceph", "cephfs-shared", 1, "osd", 1)

	safe, found, err := unstructured.NestedBool(u.Object, "spec", "metadataPool", "replicated", "requireSafeReplicaSize")
	require.NoError(t, err)
	require.True(t, found)
	assert.False(t, safe)

	pools, _, _ := unstructured.NestedSlice(u.Object, "spec", "dataPools")
	data := pools[0].(map[string]any)
	assert.Equal(t, false, data["replicated"].(map[string]any)["requireSafeReplicaSize"])
}
