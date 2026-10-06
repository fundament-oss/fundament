package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/stretchr/testify/assert"
)

func TestRenderStorageClass(t *testing.T) {
	sc := RenderStorageClass("pool-a", "rook-ceph", "pool-a", "rook-ceph")
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

// The driver name follows the *operator's* namespace (FUNP_ROOK_NAMESPACE), set
// independently of the CephCluster's. Hardcoding it yields a StorageClass no
// driver answers, whose only symptom is a PVC stuck Pending.
func TestRenderStorageClassFollowsRookNamespace(t *testing.T) {
	t.Parallel()
	sc := RenderStorageClass("pool-a", "ceph-cluster", "pool-a", "rook-system")

	assert.Equal(t, "rook-system.rbd.csi.ceph.com", sc.Provisioner)
	// clusterID and the CSI secrets track the CephCluster's namespace instead.
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

func TestRenderCephFSStorageClass(t *testing.T) {
	t.Parallel()
	sc := RenderCephFSStorageClass("cephfs-shared", "rook-ceph", "cephfs-shared", "rook-ceph", "")
	assert.Equal(t, "cephfs-shared", sc.Name)
	assert.Equal(t, "rook-ceph.cephfs.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "rook-ceph", sc.Parameters["clusterID"])
	assert.Equal(t, "cephfs-shared", sc.Parameters["fsName"])
	assert.Equal(t, "cephfs-shared-data0", sc.Parameters["pool"], "must match Rook's <fsName>-<dataPool.name>")
	assert.NotContains(t, sc.Parameters, "mounter", "empty mounter leaves the CSI default")
	assert.NotContains(t, sc.Parameters, "csi.storage.k8s.io/fstype", "CephFS is already a filesystem")
	assert.Equal(t, "rook-csi-cephfs-provisioner", sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"])
	assert.Equal(t, "rook-csi-cephfs-node", sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"])
	assert.Equal(t, "rook-csi-cephfs-node", sc.Parameters["csi.storage.k8s.io/node-publish-secret-name"])
	assert.Equal(t, "rook-csi-cephfs-provisioner", sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"])
	if assert.NotNil(t, sc.ReclaimPolicy) {
		assert.Equal(t, corev1.PersistentVolumeReclaimDelete, *sc.ReclaimPolicy)
	}
	if assert.NotNil(t, sc.AllowVolumeExpansion) {
		assert.True(t, *sc.AllowVolumeExpansion)
	}
}

// Same namespace split as RBD: driver name follows the operator's namespace,
// clusterID and secrets follow the CephCluster's.
func TestRenderCephFSStorageClassFollowsRookNamespace(t *testing.T) {
	t.Parallel()
	sc := RenderCephFSStorageClass("cephfs-shared", "ceph-cluster", "cephfs-shared", "rook-system", "")
	assert.Equal(t, "rook-system.cephfs.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "ceph-cluster", sc.Parameters["clusterID"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/node-publish-secret-namespace"])
	assert.Equal(t, "ceph-cluster", sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"])
}

// mounter=fuse serves CephFS on nodes without the ceph kernel module (local
// Docker VMs); it must reach the StorageClass verbatim.
func TestRenderCephFSStorageClassSetsFuseMounter(t *testing.T) {
	t.Parallel()
	sc := RenderCephFSStorageClass("cephfs-shared", "rook-ceph", "cephfs-shared", "rook-ceph", "fuse")
	assert.Equal(t, "fuse", sc.Parameters["mounter"])
}

func TestCephFSProvisioner(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "rook-ceph.cephfs.csi.ceph.com", CephFSProvisioner("rook-ceph"))
}

// The bucket class drives Rook's OBC provisioner, not a CSI driver: no CSI
// secret parameters, no volume expansion.
func TestRenderBucketStorageClass(t *testing.T) {
	t.Parallel()
	sc := RenderBucketStorageClass("cephobj-main", "rook-ceph", "cephobj-main", "rook-ceph")
	assert.Equal(t, "cephobj-main", sc.Name)
	assert.Equal(t, "rook-ceph.ceph.rook.io/bucket", sc.Provisioner)
	assert.Equal(t, "cephobj-main", sc.Parameters["objectStoreName"])
	assert.Equal(t, "rook-ceph", sc.Parameters["objectStoreNamespace"])
	assert.NotContains(t, sc.Parameters, "clusterID")
	// An empty BUCKET_REGION in the claim's ConfigMap breaks S3 clients that
	// require one (distribution/registry panics on it); parameters are
	// immutable, so it must ship from the start. us-east-1 is the ODF
	// convention and RGW's implicit default zonegroup alias.
	assert.Equal(t, "us-east-1", sc.Parameters["region"])
	assert.Nil(t, sc.AllowVolumeExpansion, "buckets are not volumes")
	if assert.NotNil(t, sc.ReclaimPolicy) {
		assert.Equal(t, corev1.PersistentVolumeReclaimDelete, *sc.ReclaimPolicy)
	}
	// Pinned, or server defaulting reads back as immutable drift; the
	// renderer documents why.
	if assert.NotNil(t, sc.VolumeBindingMode) {
		assert.Equal(t, storagev1.VolumeBindingImmediate, *sc.VolumeBindingMode)
	}
}

// Unlike the CSI classes, the provisioner follows the CephCluster's
// namespace (see BucketProvisioner); the two only coincide while both
// default to rook-ceph.
func TestRenderBucketStorageClassFollowsClusterNamespace(t *testing.T) {
	t.Parallel()
	sc := RenderBucketStorageClass("cephobj-main", "ceph-cluster", "cephobj-main", "rook-system")
	assert.Equal(t, "ceph-cluster.ceph.rook.io/bucket", sc.Provisioner)
	assert.Equal(t, "ceph-cluster", sc.Parameters["objectStoreNamespace"])
}

func TestBucketProvisioner(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "rook-ceph.ceph.rook.io/bucket", BucketProvisioner("rook-ceph"))
	assert.Equal(t, "ceph-cluster.ceph.rook.io/bucket", BucketProvisioner("ceph-cluster"))
}
