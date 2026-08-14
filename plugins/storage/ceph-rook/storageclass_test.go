package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/stretchr/testify/assert"
)

func TestRenderStorageClass(t *testing.T) {
	var sc *storagev1.StorageClass = RenderStorageClass("pool-a", "rook-ceph", "pool-a", "rook-ceph")
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

// Rook names its CSI drivers after the namespace the *operator* runs in, which
// FUNP_ROOK_NAMESPACE controls independently of the CephCluster's namespace.
// Hardcoding "rook-ceph" here produced a StorageClass no driver answers, and the
// symptom is a PVC that stays Pending with nothing to attribute it to.
func TestRenderStorageClassFollowsRookNamespace(t *testing.T) {
	t.Parallel()
	sc := RenderStorageClass("pool-a", "ceph-cluster", "pool-a", "rook-system")

	assert.Equal(t, "rook-system.rbd.csi.ceph.com", sc.Provisioner)
	// clusterID and every CSI secret namespace track the CephCluster's namespace,
	// not the operator's -- the two are separate knobs.
	assert.Equal(t, "ceph-cluster", sc.Parameters["clusterID"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/node-publish-secret-namespace"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"])
}

func TestRBDProvisioner(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "rook-ceph.rbd.csi.ceph.com", RBDProvisioner("rook-ceph"))
	assert.Equal(t, "storage.rbd.csi.ceph.com", RBDProvisioner("storage"))
}
