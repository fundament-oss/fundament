package main

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// RookValues returns the Helm --set values for the rook-ceph operator chart.
// The enableDiscoveryDaemon value causes the operator to deploy a DaemonSet
// that discovers raw block devices on every node, making them available as
// Disk objects.
func RookValues() map[string]string {
	return map[string]string{
		"enableDiscoveryDaemon": "true",
	}
}

// BootstrapCephCluster returns an unstructured CephCluster resource that
// establishes the baseline Ceph cluster in the given namespace. The object is
// suitable for server-side apply: it pins the Ceph image, sets quorum counts,
// disables the dashboard, and leaves storage.nodes empty so the reconciler
// (Task 9/10) can manage disk assignments independently.
func BootstrapCephCluster(namespace string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ceph.rook.io/v1",
			"kind":       "CephCluster",
			"metadata": map[string]any{
				"name":      "rook-ceph",
				"namespace": namespace,
			},
			"spec": map[string]any{
				"cephVersion": map[string]any{
					"image": "quay.io/ceph/ceph:v18.2.4",
				},
				"mon": map[string]any{
					"count": int64(3),
				},
				"mgr": map[string]any{
					"count": int64(2),
				},
				"dashboard": map[string]any{
					"enabled": false,
				},
				"storage": map[string]any{
					"useAllDevices": false,
					"nodes":         []any{},
				},
			},
		},
	}
	return u
}
