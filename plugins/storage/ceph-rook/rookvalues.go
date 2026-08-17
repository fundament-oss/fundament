package main

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

const (
	cephAPIVersion = "ceph.rook.io/v1"
	// One Ceph cluster per Kubernetes cluster; pools share its OSD set.
	cephClusterName = "rook-ceph"
)

// RookValues returns the Helm --set values for the rook-ceph operator chart.
// enableDiscoveryDaemon deploys the DaemonSet that finds raw block devices.
//
// No device denylist here: discoverDaemonUdev only filters which udev events
// trigger a re-probe, and probeDevices writes every device it finds regardless.
// ParseDiscoveredDevices is what keeps a node's real disks out of the inventory.
func RookValues(cfg Config) map[string]string {
	values := map[string]string{
		"enableDiscoveryDaemon": "true",
	}
	if cfg.DevLoopDevices {
		values["allowLoopDevices"] = "true"
	}
	return values
}

// BootstrapCephCluster returns the baseline CephCluster. storage.nodes is left
// empty so StoragePoolReconciler owns disk assignment.
func BootstrapCephCluster(namespace string, cfg Config) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": cephAPIVersion,
			"kind":       "CephCluster",
			"metadata": map[string]any{
				"name":      cephClusterName,
				"namespace": namespace,
			},
			"spec": map[string]any{
				// Mon databases, keyrings and config -- not OSD data. Rook
				// rejects a non-external cluster without it.
				"dataDirHostPath": "/var/lib/rook",
				"cephVersion": map[string]any{
					"image": cfg.CephImage,
					// Rook checks the release, not the patch level, so the
					// default v19.x image is supported and this stays false.
					// Only a release outside Rook's table (v20+) needs it.
					"allowUnsupported": cfg.AllowUnsupportedCeph,
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
					// Must stay false: a privileged node container sees the
					// host's real disks, and the reconciler names every device
					// explicitly under nodes.
					"useAllNodes":   false,
					"useAllDevices": false,
					"nodes":         []any{},
				},
			},
		},
	}
	return u
}
