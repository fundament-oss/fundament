package main

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBDProvisioner is the CSI driver name for a Rook operator installed in
// rookNamespace.
//
// Rook registers its CSI drivers as "<operator namespace>.rbd.csi.ceph.com", so
// this must follow FUNP_ROOK_NAMESPACE and not the CephCluster's namespace --
// the two are configured separately and only happen to share a default. A
// StorageClass naming a driver nobody registered does not fail: PVCs simply sit
// in Pending forever with nothing to attribute it to.
func RBDProvisioner(rookNamespace string) string {
	return rookNamespace + ".rbd.csi.ceph.com"
}

// RenderStorageClass constructs a Kubernetes StorageClass for Rook Ceph RBD provisioning.
// It configures the standard Rook RBD CSI parameters with the given cluster namespace
// and block pool name.
//
// clusterNamespace is where the CephCluster and its CSI secrets live (it is also
// the CSI clusterID); rookNamespace is where the operator runs, which is what
// names the driver.
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
