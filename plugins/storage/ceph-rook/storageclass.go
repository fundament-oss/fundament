package main

import (
	"maps"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// RBDProvisioner is the CSI driver name for a Rook operator in rookNamespace.
//
// Rook registers drivers as "<operator namespace>.rbd.csi.ceph.com", so this
// follows FUNP_ROOK_NAMESPACE, not the CephCluster's -- the two only share a
// default. Naming an unregistered driver does not fail; PVCs just sit Pending.
func RBDProvisioner(rookNamespace string) string {
	return rookNamespace + ".rbd.csi.ceph.com"
}

// CephFSProvisioner is the CephFS CSI driver name for a Rook operator in
// rookNamespace; same registration rule as RBDProvisioner.
func CephFSProvisioner(rookNamespace string) string {
	return rookNamespace + ".cephfs.csi.ceph.com"
}

// cephStorageClass builds the scaffold both CSI StorageClasses share; the
// per-driver parameters are merged on top. secretInfix picks the Rook-managed
// CSI Secrets ("rbd" or "cephfs"), so a secret-name change cannot land in one
// renderer and miss the other.
func cephStorageClass(name, provisioner, clusterNamespace, secretInfix string, params map[string]string) *storagev1.StorageClass {
	// The csi.storage.k8s.io/*-secret-name keys name the Rook-managed Secrets
	// the CSI driver looks up; no credential is in this file.
	shared := map[string]string{ //nolint:gosec // G101: CSI parameter names, not credentials
		"clusterID": clusterNamespace,
		"csi.storage.k8s.io/provisioner-secret-name":            "rook-csi-" + secretInfix + "-provisioner",
		"csi.storage.k8s.io/provisioner-secret-namespace":       clusterNamespace,
		"csi.storage.k8s.io/node-stage-secret-name":             "rook-csi-" + secretInfix + "-node",
		"csi.storage.k8s.io/node-stage-secret-namespace":        clusterNamespace,
		"csi.storage.k8s.io/node-publish-secret-name":           "rook-csi-" + secretInfix + "-node",
		"csi.storage.k8s.io/node-publish-secret-namespace":      clusterNamespace,
		"csi.storage.k8s.io/controller-expand-secret-name":      "rook-csi-" + secretInfix + "-provisioner",
		"csi.storage.k8s.io/controller-expand-secret-namespace": clusterNamespace,
	}
	maps.Copy(shared, params)
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:          provisioner,
		ReclaimPolicy:        ptr.To(corev1.PersistentVolumeReclaimDelete),
		Parameters:           shared,
		AllowVolumeExpansion: new(true),
		VolumeBindingMode:    ptr.To(storagev1.VolumeBindingImmediate),
	}
}

// RenderStorageClass builds the RBD StorageClass. clusterNamespace holds the
// CephCluster and its CSI secrets and is the clusterID; rookNamespace runs the
// operator and names the driver.
func RenderStorageClass(name, clusterNamespace, blockPoolName, rookNamespace string) *storagev1.StorageClass {
	return cephStorageClass(name, RBDProvisioner(rookNamespace), clusterNamespace, "rbd", map[string]string{
		"pool": blockPoolName,
		// The full krbd feature set; kernels >= 5.4 support all of these.
		// StorageClass parameters are immutable, so widening later would
		// mean recreating the class.
		"imageFeatures":             "layering,exclusive-lock,object-map,fast-diff,deep-flatten",
		"csi.storage.k8s.io/fstype": "ext4",
	})
}

// RenderCephFSStorageClass builds the CephFS StorageClass. fsName is the
// CephFilesystem's name; the pool parameter must name Rook's derived data pool
// or provisioning fails. mounter selects the CSI mount method ("" = CSI default
// kernel client, "fuse" = ceph-fuse for nodes without the ceph kernel module).
func RenderCephFSStorageClass(name, clusterNamespace, fsName, rookNamespace, mounter string) *storagev1.StorageClass {
	params := map[string]string{
		"fsName": fsName,
		"pool":   fsName + "-" + cephFSDataPoolName,
	}
	if mounter != "" {
		params["mounter"] = mounter
	}
	return cephStorageClass(name, CephFSProvisioner(rookNamespace), clusterNamespace, "cephfs", params)
}
