package main

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RenderStorageClass constructs a Kubernetes StorageClass for Rook Ceph RBD provisioning.
// It configures the standard Rook RBD CSI parameters with the given cluster namespace
// and block pool name.
func RenderStorageClass(name, clusterNamespace, blockPoolName string) *storagev1.StorageClass {
	reclaim := corev1.PersistentVolumeReclaimDelete
	allowExpansion := true

	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:   "rook-ceph.rbd.csi.ceph.com",
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
