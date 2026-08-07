package main

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// RookValues returns the Helm --set values for the rook-ceph operator chart.
// The enableDiscoveryDaemon value causes the operator to deploy a DaemonSet
// that discovers raw block devices on every node, making them available as
// Disk objects.
func RookValues(allowLoopDevices bool) map[string]string {
	v := map[string]string{
		"enableDiscoveryDaemon": "true",
	}
	if allowLoopDevices {
		// Chart value: permit loop-backed devices as OSDs (dev/CI only). Also
		// makes the discover daemon surface loop devices so they appear in the
		// Disk inventory.
		v["allowLoopDevices"] = "true"
	}
	return v
}

// monCountForNodes picks an odd mon count that fits the cluster size: 3 for a
// real multi-node cluster (HA quorum), 1 for a 1–2 node cluster, so mons always
// schedule without allowMultiplePerNode (Rook refuses 3 mons on <3 nodes).
func monCountForNodes(nodeCount int) int {
	if nodeCount >= 3 {
		return 3
	}
	return 1
}

// mgrCountForNodes returns 2 mgrs (active+standby) on a multi-node cluster, else
// 1 so the mgr schedules on a single node.
func mgrCountForNodes(nodeCount int) int {
	if nodeCount >= 2 {
		return 2
	}
	return 1
}

// BootstrapCephCluster returns an unstructured CephCluster resource that
// establishes the baseline Ceph cluster in the given namespace. The object is
// suitable for server-side apply: it pins the Ceph image, sizes the mon/mgr
// counts from nodeCount so the control plane schedules on small/dev clusters as
// well as production ones, disables the dashboard, and leaves storage.nodes
// empty so the reconciler manages disk assignments independently.
func BootstrapCephCluster(namespace string, nodeCount int) *unstructured.Unstructured {
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
					"count": int64(monCountForNodes(nodeCount)),
				},
				"mgr": map[string]any{
					"count": int64(mgrCountForNodes(nodeCount)),
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
