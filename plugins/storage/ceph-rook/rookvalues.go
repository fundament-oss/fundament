package main

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

const (
	// cephAPIVersion is the Rook CRD group/version for every object this
	// plugin reads or writes through the unstructured client.
	cephAPIVersion = "ceph.rook.io/v1"
	// cephClusterName is the singleton CephCluster's name. The plugin runs one
	// Ceph cluster per Kubernetes cluster; tiering is done with multiple
	// CephBlockPools over the shared OSD set.
	cephClusterName = "rook-ceph"
)

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
			"apiVersion": cephAPIVersion,
			"kind":       "CephCluster",
			"metadata": map[string]any{
				"name":      cephClusterName,
				"namespace": namespace,
			},
			"spec": map[string]any{
				// Mon databases, keyrings and config -- not OSD data. Rook has
				// no default and rejects a non-external cluster without it.
				"dataDirHostPath": "/var/lib/rook",
				"cephVersion": map[string]any{
					"image": cfg.CephImage,
					// A Ceph release Rook does not list as supported is
					// rejected outright, which is what a pinned image newer
					// than the chart's table hits. deploy/k3d/rook-smoke.sh
					// needs the same flag against the same pair, so leaving it
					// off here would make the plugin fail where the smoke test
					// that validates the environment succeeds.
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
					// A privileged node container enumerates the host's real
					// disks, so both of these must stay false: the StoragePool
					// reconciler names every device explicitly under nodes.
					"useAllNodes":   false,
					"useAllDevices": false,
					"nodes":         []any{},
				},
			},
		},
	}
	return u
}
