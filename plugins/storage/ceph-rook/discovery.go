package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// rawDevice mirrors the sys.LocalDisk fields this plugin reads. Tags must match
// that struct exactly: rook serialises it straight into the ConfigMap, and a tag
// that does not exist there decodes to the zero value in silence.
type rawDevice struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Rotational bool   `json:"rotational"`
	Type       string `json:"type"`
	Empty      bool   `json:"empty"`
	Filesystem string `json:"filesystem"`
	Vendor     string `json:"vendor"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	WWN        string `json:"wwn"`
	// DevLinks is a space-separated list of udev symlinks. It is the only place
	// rook reports stable names; sys.LocalDisk has no "by-id" field.
	DevLinks string `json:"devLinks"`
}

// loopPartition matches a partition on a loop device, e.g. /dev/loop0p1.
var loopPartition = regexp.MustCompile(`^/dev/loop\d+p\d+$`)

const byIDDir = "/dev/disk/by-id/"

// byIDPreference ranks /dev/disk/by-id link forms, most preferred first. Each is
// derived from something burned into the device, so it survives a rename.
//
// Forms outside this list are ignored rather than used as a last resort:
// lvm-pv-uuid-*, md-uuid-* and dm-* name a logical volume, which can be rebuilt
// on another physical device and would point the pool at the wrong disk.
var byIDPreference = []string{
	"wwn-",
	"nvme-eui.",
	"scsi-",
	"ata-",
	"nvme-",
	"virtio-",
	"usb-",
}

// byIDPath picks the most stable /dev/disk/by-id symlink from devLinks, or ""
// when the device has none (loop devices, some virtual disks).
func byIDPath(devLinks string) string {
	best, bestRank := "", len(byIDPreference)
	for _, link := range strings.Fields(devLinks) {
		if !strings.HasPrefix(link, byIDDir) {
			continue
		}
		suffix := strings.TrimPrefix(link, byIDDir)
		for rank, prefix := range byIDPreference {
			if !strings.HasPrefix(suffix, prefix) {
				continue
			}
			// Lower rank wins; ties break lexicographically, since udev's
			// ordering is not stable between probes.
			if rank < bestRank || (rank == bestRank && link < best) {
				best, bestRank = link, rank
			}
			break
		}
	}
	return best
}

// isNVMe reports whether a device is NVMe-attached. sys.LocalDisk has no
// transport field, so this goes on the kernel name and the by-id link form.
func isNVMe(d *rawDevice) bool {
	if strings.HasPrefix(d.Name, "nvme") {
		return true
	}
	for link := range strings.FieldsSeq(d.DevLinks) {
		if strings.HasPrefix(link, byIDDir+"nvme-") {
			return true
		}
	}
	return false
}

// ParseDiscoveredDevices converts a discover ConfigMap's device JSON into
// candidate Disk statuses.
//
// Only whole disks (type=="disk"), or in loop-device mode only loop-backed
// partitions -- replaced, not extended. A k3d node's privileged containers see
// the host's real disks and they do reach the ConfigMap, so this filter is the
// only thing keeping them out of an OSD.
func ParseDiscoveredDevices(node, raw string, loopDevices bool) ([]v1alpha1.DiskStatus, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var devs []rawDevice
	if err := json.Unmarshal([]byte(raw), &devs); err != nil {
		return nil, fmt.Errorf("parse device json for node %q: %w", node, err)
	}
	out := make([]v1alpha1.DiskStatus, 0, len(devs))
	for i := range devs {
		d := &devs[i]
		path := "/dev/" + d.Name
		if loopDevices {
			// Loop-backed partitions report as "part"; a bare loop device is
			// rejected by the discover daemon itself.
			if d.Type != "part" || !loopPartition.MatchString(path) {
				continue
			}
		} else if d.Type != "disk" {
			continue
		}
		dt := v1alpha1.DiskTypeSSD
		switch {
		case d.Rotational:
			dt = v1alpha1.DiskTypeHDD
		case isNVMe(d):
			dt = v1alpha1.DiskTypeNVMe
		}
		out = append(out, v1alpha1.DiskStatus{
			Node:       node,
			Path:       path,
			StablePath: byIDPath(d.DevLinks),
			SizeBytes:  d.Size,
			Type:       dt,
			Rotational: d.Rotational,
			Model:      d.Model,
			Serial:     d.Serial,
			WWN:        d.WWN,
			Available:  d.Empty && d.Filesystem == "",
		})
	}
	return out, nil
}

// DeviceKey is the identity a Disk CR is named after, preferring whatever
// survives a reboot: a Disk whose name changes drops out of every StoragePool
// listing it, and the device silently leaves the CephCluster.
//
// The prefixes stop a serial colliding with a WWN of the same text.
func DeviceKey(st *v1alpha1.DiskStatus) string {
	switch {
	case st.StablePath != "":
		return st.StablePath
	case st.WWN != "":
		return "wwn:" + st.WWN
	case st.Serial != "":
		return "serial:" + st.Serial
	default:
		// Nothing stable reported -- loop and some virtio disks. The name then
		// moves if the kernel reorders them.
		return "path:" + st.Path
	}
}

// A node name may be a full 253-character DNS subdomain; leaving room for the
// digest keeps the result inside the API server's name limit.
const maxDiskNameBase = 200

// DiskName is a deterministic, DNS-1123-safe name for a (node, key) pair, where
// key comes from DeviceKey.
//
// The digest covers the node, not just the key. The readable prefix cannot carry
// the node on its own: the sanitiser below folds dots to dashes, so "worker.1"
// and "worker-1" collide. Devices with no stable identity all key on
// "path:/dev/sdb", so hashing the key alone would collapse two nodes' disks onto
// one CR.
//
// sha256 rather than sha1 only to avoid a linter exemption; this is a naming
// device, not a security control.
func DiskName(node, key string) string {
	sum := sha256.Sum256([]byte(node + "\x00" + key))
	short := hex.EncodeToString(sum[:])[:10]
	base := strings.ToLower(node)
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, base)
	if len(base) > maxDiskNameBase {
		base = base[:maxDiskNameBase]
	}
	// After truncating too: a cut can strand a trailing dash.
	base = strings.Trim(base, "-")
	if base == "" {
		base = "node"
	}
	return fmt.Sprintf("%s-%s", base, short)
}
