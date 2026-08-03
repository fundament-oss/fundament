package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

const sampleDevices = `[
 {"name":"sdb","size":1073741824,"rotational":1,"type":"disk","empty":true,"filesystem":"","vendor":"ATA","model":"DISK1","serial":"S1","by-id":"/dev/disk/by-id/wwn-0xAAA"},
 {"name":"sdc","size":2147483648,"rotational":0,"type":"disk","empty":false,"filesystem":"ext4","by-id":"/dev/disk/by-id/wwn-0xBBB"},
 {"name":"sda1","size":500,"rotational":1,"type":"part","empty":false}
]`

func TestParseDiscoveredDevices(t *testing.T) {
	got, err := ParseDiscoveredDevices("node-1", sampleDevices)
	require.NoError(t, err)
	require.Len(t, got, 2) // the "part" entry is skipped

	assert.Equal(t, "node-1", got[0].Node)
	assert.Equal(t, "/dev/disk/by-id/wwn-0xAAA", got[0].Path)
	assert.Equal(t, int64(1073741824), got[0].SizeBytes)
	assert.Equal(t, v1alpha1.DiskTypeHDD, got[0].Type) // rotational=1
	assert.True(t, got[0].Rotational)
	assert.True(t, got[0].Available) // empty && no filesystem

	assert.Equal(t, v1alpha1.DiskTypeSSD, got[1].Type) // rotational=0
	assert.False(t, got[1].Available)                  // has filesystem
}

func TestDiskNameIsDeterministicAndDNSSafe(t *testing.T) {
	n1 := DiskName("Node-1", "/dev/disk/by-id/wwn-0xAAA")
	n2 := DiskName("Node-1", "/dev/disk/by-id/wwn-0xAAA")
	assert.Equal(t, n1, n2)
	assert.Regexp(t, `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, n1)
}
