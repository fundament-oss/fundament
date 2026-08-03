package main

import (
	"sort"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// BuildStorageNodes constructs the storage nodes configuration for CephCluster.spec.storage.nodes
// from a list of disk statuses. Disks are grouped by node, sorted deterministically,
// and formatted as a slice of maps containing node name and device paths.
func BuildStorageNodes(disks []v1alpha1.DiskStatus) []map[string]any {
	if len(disks) == 0 {
		return nil
	}

	// Group disks by node
	nodeMap := make(map[string][]string)
	for _, disk := range disks {
		nodeMap[disk.Node] = append(nodeMap[disk.Node], disk.Path)
	}

	// Sort node names
	nodes := make([]string, 0, len(nodeMap))
	for node := range nodeMap {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	// Build the result slice
	result := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		paths := nodeMap[node]
		// Sort device paths for each node
		sort.Strings(paths)

		// Build devices slice
		devices := make([]map[string]any, 0, len(paths))
		for _, path := range paths {
			devices = append(devices, map[string]any{
				"name": path,
			})
		}

		// Add node entry
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
	for _, disk := range disks {
		nodeSet[disk.Node] = struct{}{}
	}

	return len(nodeSet)
}
