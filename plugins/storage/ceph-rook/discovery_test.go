package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

const sampleDevices = `[
 {"name":"sdb","size":1073741824,"rotational":true,"type":"disk","empty":true,"filesystem":"","vendor":"ATA","model":"DISK1","serial":"S1","by-id":"/dev/disk/by-id/wwn-0xAAA"},
 {"name":"sdc","size":2147483648,"rotational":false,"type":"disk","empty":false,"filesystem":"ext4","by-id":"/dev/disk/by-id/wwn-0xBBB"},
 {"name":"sda1","size":500,"rotational":true,"type":"part","empty":false}
]`

// What rook-discover reports on a k3d node prepared by
// deploy/k3d/storage-disks.sh: the host's own partitions alongside the
// loop-backed ones, and no by-id field at all.
const sampleLoopDevices = `[
 {"name":"vda1","parent":"vda","size":20400029184,"rotational":true,"type":"part","empty":false,"filesystem":"ext4","real-path":"/dev/vda1","kernel-name":"vda1"},
 {"name":"vdc","size":19943424,"rotational":true,"type":"disk","empty":false,"filesystem":"iso9660","real-path":"/dev/vdc","kernel-name":"vdc"},
 {"name":"loop0p1","parent":"loop0","size":21472739328,"rotational":true,"type":"part","empty":true,"filesystem":"","real-path":"/dev/loop0p1","kernel-name":"loop0p1"},
 {"name":"loop1p1","parent":"loop1","size":21472739328,"rotational":true,"type":"part","empty":true,"filesystem":"","real-path":"/dev/loop1p1","kernel-name":"loop1p1"}
]`

func TestParseDiscoveredDevices(t *testing.T) {
	got, err := ParseDiscoveredDevices("node-1", sampleDevices, false)
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

func TestParseDiscoveredDevicesLoopMode(t *testing.T) {
	got, err := ParseDiscoveredDevices("node-1", sampleLoopDevices, true)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "/dev/loop0p1", got[0].Path)
	assert.Equal(t, "/dev/loop1p1", got[1].Path)
	assert.True(t, got[0].Available)
}

func TestParseDiscoveredDevicesLoopModeRejectsRealDevices(t *testing.T) {
	got, err := ParseDiscoveredDevices("node-1", sampleDevices, true)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// A loop partition carrying a filesystem is discovered but not offered up.
func TestParseDiscoveredDevicesLoopModeMarksFormattedUnavailable(t *testing.T) {
	const formatted = `[
 {"name":"loop0p1","size":21472739328,"rotational":true,"type":"part","empty":false,"filesystem":"ext4"}
]`
	got, err := ParseDiscoveredDevices("node-1", formatted, true)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.False(t, got[0].Available)
}

func TestParseDiscoveredDevicesRejectsLoopWhenDisabled(t *testing.T) {
	got, err := ParseDiscoveredDevices("node-1", sampleLoopDevices, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "/dev/vdc", got[0].Path) // the only type=="disk" entry
}

func TestDiskNameIsDeterministicAndDNSSafe(t *testing.T) {
	n1 := DiskName("Node-1", "/dev/disk/by-id/wwn-0xAAA")
	n2 := DiskName("Node-1", "/dev/disk/by-id/wwn-0xAAA")
	assert.Equal(t, n1, n2)
	assert.Regexp(t, `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, n1)
}
