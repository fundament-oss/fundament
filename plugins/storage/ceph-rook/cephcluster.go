package main

import (
	"sort"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// DeviceRef is what goes into CephCluster spec.storage.nodes[].devices[].name.
// Rook takes a kernel name or a udev path; the by-id form is the one that
// survives a reboot, and a renamed device would take its OSD out of the cluster.
// Disks with no by-id link fall back to the kernel path.
func DeviceRef(disk *v1alpha1.DiskStatus) string {
	if disk.StablePath != "" {
		return disk.StablePath
	}
	return disk.Path
}

// BuildStorageNodes builds CephCluster spec.storage.nodes, grouped by node and
// sorted so repeated calls produce identical output.
func BuildStorageNodes(disks []v1alpha1.DiskStatus) []map[string]any {
	if len(disks) == 0 {
		return nil
	}

	nodeMap := make(map[string][]string)
	for i := range disks {
		disk := &disks[i]
		nodeMap[disk.Node] = append(nodeMap[disk.Node], DeviceRef(disk))
	}

	nodes := make([]string, 0, len(nodeMap))
	for node := range nodeMap {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	result := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		paths := nodeMap[node]
		sort.Strings(paths)

		devices := make([]map[string]any, 0, len(paths))
		for _, path := range paths {
			devices = append(devices, map[string]any{
				"name": path,
			})
		}

		result = append(result, map[string]any{
			"name":    node,
			"devices": devices,
		})
	}

	return result
}

// DistinctNodeCount returns the number of unique nodes in the disk list.
func DistinctNodeCount(disks []v1alpha1.DiskStatus) int {
	if len(disks) == 0 {
		return 0
	}

	nodeSet := make(map[string]struct{})
	for i := range disks {
		nodeSet[disks[i].Node] = struct{}{}
	}

	return len(nodeSet)
}
