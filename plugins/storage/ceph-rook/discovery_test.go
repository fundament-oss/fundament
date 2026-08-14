package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// Shaped like rook's sys.LocalDisk, which is what rook-discover serialises into
// the ConfigMap. Note "devLinks": there is no "by-id" key on that struct, so a
// tag naming one decodes to "" and every device silently falls back to its
// kernel name.
const sampleDevices = `[
 {"name":"sdb","size":1073741824,"rotational":true,"type":"disk","empty":true,"filesystem":"","vendor":"ATA","model":"DISK1","serial":"S1","wwn":"0x5000c500a1b2c3d4","devLinks":"/dev/disk/by-id/wwn-0x5000c500a1b2c3d4 /dev/disk/by-id/ata-DISK1_S1 /dev/disk/by-path/pci-0000:00:17.0-ata-1"},
 {"name":"sdc","size":2147483648,"rotational":false,"type":"disk","empty":false,"filesystem":"ext4","devLinks":"/dev/disk/by-id/ata-DISK2_S2"},
 {"name":"sda1","size":500,"rotational":true,"type":"part","empty":false}
]`

// What rook-discover reports on a k3d node prepared by
// deploy/k3d/storage-disks.sh: the host's own partitions alongside the
// loop-backed ones, and no stable links at all.
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
	assert.Equal(t, "/dev/sdb", got[0].Path, "Path is the kernel name")
	assert.Equal(t, "/dev/disk/by-id/wwn-0x5000c500a1b2c3d4", got[0].StablePath)
	assert.Equal(t, "0x5000c500a1b2c3d4", got[0].WWN)
	assert.Equal(t, int64(1073741824), got[0].SizeBytes)
	assert.Equal(t, v1alpha1.DiskTypeHDD, got[0].Type) // rotational=1
	assert.True(t, got[0].Rotational)
	assert.True(t, got[0].Available) // empty && no filesystem

	assert.Equal(t, v1alpha1.DiskTypeSSD, got[1].Type) // rotational=0
	assert.False(t, got[1].Available)                  // has filesystem
}

// The whole reason StablePath exists: a kernel rename must not change what the
// CephCluster is pointed at, nor what the Disk CR is called.
func TestParseDiscoveredDevicesSurvivesKernelRename(t *testing.T) {
	t.Parallel()
	const before = `[{"name":"sdb","type":"disk","empty":true,"size":100,"wwn":"0xABC","devLinks":"/dev/disk/by-id/wwn-0xABC"}]`
	const after = `[{"name":"sdc","type":"disk","empty":true,"size":100,"wwn":"0xABC","devLinks":"/dev/disk/by-id/wwn-0xABC"}]`

	first, err := ParseDiscoveredDevices("node-1", before, false)
	require.NoError(t, err)
	second, err := ParseDiscoveredDevices("node-1", after, false)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, second, 1)

	assert.NotEqual(t, first[0].Path, second[0].Path, "the kernel name did move")
	assert.Equal(t, DeviceRef(first[0]), DeviceRef(second[0]),
		"the CephCluster device entry must not move with it")
	assert.Equal(t, DiskName("node-1", DeviceKey(first[0])), DiskName("node-1", DeviceKey(second[0])),
		"a renamed Disk CR would drop out of every StoragePool listing it")
}

func TestByIDPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		devLinks string
		want     string
	}{
		{"empty", "", ""},
		{
			name:     "wwn beats ata",
			devLinks: "/dev/disk/by-id/ata-DISK_S1 /dev/disk/by-id/wwn-0xABC",
			want:     "/dev/disk/by-id/wwn-0xABC",
		},
		{
			name:     "nvme-eui beats the model form",
			devLinks: "/dev/disk/by-id/nvme-SAMSUNG_MZ_S1 /dev/disk/by-id/nvme-eui.0025385",
			want:     "/dev/disk/by-id/nvme-eui.0025385",
		},
		{
			name:     "by-path is not a stable id",
			devLinks: "/dev/disk/by-path/pci-0000:00:17.0-ata-1",
			want:     "",
		},
		{
			name: "logical-volume links are ignored, not used as a fallback",
			// lvm-pv-uuid names a PV that can be rebuilt onto another disk, so
			// following it would point the pool at the wrong device.
			devLinks: "/dev/disk/by-id/lvm-pv-uuid-abc123 /dev/disk/by-id/dm-name-vg0-lv0",
			want:     "",
		},
		{
			name:     "equal rank is broken lexicographically, not by udev order",
			devLinks: "/dev/disk/by-id/wwn-0xBBB /dev/disk/by-id/wwn-0xAAA",
			want:     "/dev/disk/by-id/wwn-0xAAA",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, byIDPath(tt.devLinks))
		})
	}
}

func TestParseDiscoveredDevicesDetectsNVMe(t *testing.T) {
	t.Parallel()
	const devices = `[
 {"name":"nvme0n1","size":100,"rotational":false,"type":"disk","empty":true},
 {"name":"xvdf","size":100,"rotational":false,"type":"disk","empty":true,"devLinks":"/dev/disk/by-id/nvme-Amazon_EC2_NVMe"},
 {"name":"sdb","size":100,"rotational":false,"type":"disk","empty":true},
 {"name":"sdc","size":100,"rotational":true,"type":"disk","empty":true}
]`
	got, err := ParseDiscoveredDevices("node-1", devices, false)
	require.NoError(t, err)
	require.Len(t, got, 4)

	assert.Equal(t, v1alpha1.DiskTypeNVMe, got[0].Type, "kernel name says nvme")
	assert.Equal(t, v1alpha1.DiskTypeNVMe, got[1].Type, "by-id link says nvme")
	assert.Equal(t, v1alpha1.DiskTypeSSD, got[2].Type)
	assert.Equal(t, v1alpha1.DiskTypeHDD, got[3].Type, "rotational wins over everything")
}

func TestDeviceKeyPrefersStableIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status v1alpha1.DiskStatus
		want   string
	}{
		{
			name:   "by-id link wins",
			status: v1alpha1.DiskStatus{Path: "/dev/sdb", StablePath: "/dev/disk/by-id/wwn-0xABC", WWN: "0xABC", Serial: "S1"},
			want:   "/dev/disk/by-id/wwn-0xABC",
		},
		{
			name:   "wwn when there is no link",
			status: v1alpha1.DiskStatus{Path: "/dev/sdb", WWN: "0xABC", Serial: "S1"},
			want:   "wwn:0xABC",
		},
		{
			name:   "serial when there is no wwn",
			status: v1alpha1.DiskStatus{Path: "/dev/sdb", Serial: "S1"},
			want:   "serial:S1",
		},
		{
			name:   "kernel path is the last resort",
			status: v1alpha1.DiskStatus{Path: "/dev/loop0p1"},
			want:   "path:/dev/loop0p1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DeviceKey(tt.status))
		})
	}

	// The prefixes exist so these two can never collide.
	assert.NotEqual(t,
		DeviceKey(v1alpha1.DiskStatus{WWN: "X"}),
		DeviceKey(v1alpha1.DiskStatus{Serial: "X"}))
}

func TestDeviceRef(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/dev/disk/by-id/wwn-0xABC",
		DeviceRef(v1alpha1.DiskStatus{Path: "/dev/sdb", StablePath: "/dev/disk/by-id/wwn-0xABC"}))
	// Loop devices expose nothing stable; the kernel path is all there is.
	assert.Equal(t, "/dev/loop0p1", DeviceRef(v1alpha1.DiskStatus{Path: "/dev/loop0p1"}))
}

func TestParseDiscoveredDevicesLoopMode(t *testing.T) {
	got, err := ParseDiscoveredDevices("node-1", sampleLoopDevices, true)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "/dev/loop0p1", got[0].Path)
	assert.Equal(t, "/dev/loop1p1", got[1].Path)
	assert.Empty(t, got[0].StablePath, "loop devices have no by-id link")
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
