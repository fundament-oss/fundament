package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

func TestBuildStorageNodes(t *testing.T) {
	disks := []v1alpha1.DiskStatus{
		{Node: "n2", Path: "/dev/disk/by-id/z"},
		{Node: "n1", Path: "/dev/disk/by-id/b"},
		{Node: "n1", Path: "/dev/disk/by-id/a"},
	}
	nodes := BuildStorageNodes(disks)
	assert.Equal(t, []map[string]any{
		{"name": "n1", "devices": []map[string]any{
			{"name": "/dev/disk/by-id/a"},
			{"name": "/dev/disk/by-id/b"},
		}},
		{"name": "n2", "devices": []map[string]any{
			{"name": "/dev/disk/by-id/z"},
		}},
	}, nodes)
	assert.Equal(t, 2, DistinctNodeCount(disks))
}

func TestBuildStorageNodesEmpty(t *testing.T) {
	assert.Empty(t, BuildStorageNodes(nil))
	assert.Equal(t, 0, DistinctNodeCount(nil))
}
