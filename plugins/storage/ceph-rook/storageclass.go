package main

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBDProvisioner is the CSI driver name for a Rook operator in rookNamespace.
//
// Rook registers drivers as "<operator namespace>.rbd.csi.ceph.com", so this
// follows FUNP_ROOK_NAMESPACE, not the CephCluster's -- the two only share a
// default. Naming an unregistered driver does not fail; PVCs just sit Pending.
func RBDProvisioner(rookNamespace string) string {
	return rookNamespace + ".rbd.csi.ceph.com"
}

// RenderStorageClass builds the RBD StorageClass. clusterNamespace holds the
// CephCluster and its CSI secrets and is the clusterID; rookNamespace runs the
// operator and names the driver.
func RenderStorageClass(name, clusterNamespace, blockPoolName, rookNamespace string) *storagev1.StorageClass {
	reclaim := corev1.PersistentVolumeReclaimDelete
	allowExpansion := true

	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:   RBDProvisioner(rookNamespace),
		ReclaimPolicy: &reclaim,
		Parameters: map[string]string{
			"clusterID":     clusterNamespace,
			"pool":          blockPoolName,
			"imageFeatures": "layering",
			"csi.storage.k8s.io/provisioner-secret-name":            "rook-csi-rbd-provisioner",
			"csi.storage.k8s.io/provisioner-secret-namespace":       clusterNamespace,
			"csi.storage.k8s.io/node-stage-secret-name":             "rook-csi-rbd-node",
			"csi.storage.k8s.io/node-stage-secret-namespace":        clusterNamespace,
			"csi.storage.k8s.io/node-publish-secret-name":           "rook-csi-rbd-node",
			"csi.storage.k8s.io/node-publish-secret-namespace":      clusterNamespace,
			"csi.storage.k8s.io/controller-expand-secret-name":      "rook-csi-rbd-provisioner",
			"csi.storage.k8s.io/controller-expand-secret-namespace": clusterNamespace,
			"csi.storage.k8s.io/fstype":                             "ext4",
		},
		AllowVolumeExpansion: &allowExpansion,
		VolumeBindingMode:    &[]storagev1.VolumeBindingMode{storagev1.VolumeBindingImmediate}[0],
	}
}
