package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/stretchr/testify/assert"
)

func TestRenderStorageClass(t *testing.T) {
	var sc *storagev1.StorageClass = RenderStorageClass("pool-a", "rook-ceph", "pool-a")
	assert.Equal(t, "pool-a", sc.Name)
	assert.Equal(t, "rook-ceph.rbd.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "rook-ceph", sc.Parameters["clusterID"])
	assert.Equal(t, "pool-a", sc.Parameters["pool"])
	assert.Equal(t, "ext4", sc.Parameters["csi.storage.k8s.io/fstype"])
	assert.Equal(t, "rook-csi-rbd-provisioner", sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"])
	assert.Equal(t, "rook-ceph", sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"])
	if assert.NotNil(t, sc.ReclaimPolicy) {
		assert.Equal(t, corev1.PersistentVolumeReclaimDelete, *sc.ReclaimPolicy)
	}
	if assert.NotNil(t, sc.AllowVolumeExpansion) {
		assert.True(t, *sc.AllowVolumeExpansion)
	}
}
