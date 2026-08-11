package main

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// RookValues returns the Helm --set values for the rook-ceph operator chart.
// The enableDiscoveryDaemon value causes the operator to deploy a DaemonSet
// that discovers raw block devices on every node, making them available as
// Disk objects.
//
// There is no device denylist here. Rook's discoverDaemonUdev setting filters
// which udev events trigger a re-probe; probeDevices writes every device it
// finds to the ConfigMap regardless, so it cannot keep a node's real disks out
// of the disk inventory. ParseDiscoveredDevices is the only thing that does.
func RookValues(cfg Config) map[string]string {
	values := map[string]string{
		"enableDiscoveryDaemon": "true",
	}
	if cfg.DevLoopDevices {
		values["allowLoopDevices"] = "true"
	}
	return values
}

// BootstrapCephCluster returns an unstructured CephCluster resource that
// establishes the baseline Ceph cluster in the given namespace. The object is
// suitable for server-side apply: it pins the Ceph image, sets quorum counts,
// disables the dashboard, and leaves storage.nodes empty so the reconciler
// manages disk assignments independently.
func BootstrapCephCluster(namespace string, cfg Config) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ceph.rook.io/v1",
			"kind":       "CephCluster",
			"metadata": map[string]any{
				"name":      "rook-ceph",
				"namespace": namespace,
			},
			"spec": map[string]any{
				// Mon databases, keyrings and config -- not OSD data. Rook has
				// no default and rejects a non-external cluster without it.
				"dataDirHostPath": "/var/lib/rook",
				"cephVersion": map[string]any{
					"image": cfg.CephImage,
				},
				"mon": map[string]any{
					"count":                cfg.MonCount,
					"allowMultiplePerNode": cfg.AllowMultiplePerNode,
				},
				"mgr": map[string]any{
					"count":                cfg.MgrCount,
					"allowMultiplePerNode": cfg.AllowMultiplePerNode,
				},
				"dashboard": map[string]any{
					"enabled": false,
				},
				"storage": map[string]any{
					// A privileged node container enumerates the host's real disks.
					"useAllDevices": false,
					"nodes":         []any{},
				},
			},
		},
	}
	return u
}
