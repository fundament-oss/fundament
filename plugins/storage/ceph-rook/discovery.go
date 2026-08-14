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

// rawDevice mirrors the fields of rook's sys.LocalDisk that this plugin reads.
// The JSON tags have to match that struct exactly -- rook serialises it straight
// into the discovery ConfigMap, and a tag that does not exist there decodes to
// the zero value in silence.
type rawDevice struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Rook's discover ConfigMap emits this as a JSON bool (sys.LocalDisk).
	Rotational bool   `json:"rotational"`
	Type       string `json:"type"`
	Empty      bool   `json:"empty"`
	Filesystem string `json:"filesystem"`
	Vendor     string `json:"vendor"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	WWN        string `json:"wwn"`
	// DevLinks is a space-separated list of udev symlinks (/dev/disk/by-id/...,
	// /dev/disk/by-path/...). It is the only place rook reports the stable names
	// for a device; there is no "by-id" field on sys.LocalDisk.
	DevLinks string `json:"devLinks"`
}

// loopPartition matches a partition on a loop device, e.g. /dev/loop0p1.
var loopPartition = regexp.MustCompile(`^/dev/loop\d+p\d+$`)

const byIDDir = "/dev/disk/by-id/"

// byIDPreference ranks /dev/disk/by-id link forms from most to least preferred.
// Every one of them is derived from something burned into the device, so it
// survives a reboot renaming /dev/sdb to /dev/sdc.
//
// Link forms outside this list are deliberately ignored rather than used as a
// last resort: lvm-pv-uuid-*, md-uuid-* and dm-* name a logical volume, which
// can be rebuilt on top of a different physical device and would then point the
// pool at the wrong disk.
var byIDPreference = []string{
	"wwn-",
	"nvme-eui.",
	"scsi-",
	"ata-",
	"nvme-",
	"virtio-",
	"usb-",
}

// byIDPath picks the most stable /dev/disk/by-id symlink out of rook's
// space-separated devLinks, or "" when the device has none (loop devices, and
// virtual disks whose backend exposes no identity).
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
			// Lower rank wins. Equal rank falls back to the lexicographically
			// first link so the choice never depends on udev's ordering, which
			// is not guaranteed stable between probes.
			if rank < bestRank || (rank == bestRank && link < best) {
				best, bestRank = link, rank
			}
			break
		}
	}
	return best
}

// isNVMe reports whether a device is NVMe-attached. sys.LocalDisk has no
// transport field, so this goes on the kernel name and the by-id link form,
// which are the two places the transport shows up.
func isNVMe(d rawDevice) bool {
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

// ParseDiscoveredDevices converts a rook discover ConfigMap's device JSON into
// candidate Disk statuses.
//
// Normally only whole disks (type=="disk") are returned. In loop-device mode
// that is replaced -- not extended -- by loop-backed partitions. A k3d node
// exposes the host's real disks to every privileged container and they do reach
// the discover ConfigMap, so this filter is the only thing keeping them out of
// an OSD.
//
// Path is the kernel name (/dev/sdb), which is what an operator recognises but
// is not stable across reboots. StablePath is the by-id link when the device
// has one, and is what actually gets written into the CephCluster and hashed
// into the Disk's name -- see DeviceKey and DeviceRef.
func ParseDiscoveredDevices(node string, raw string, loopDevices bool) ([]v1alpha1.DiskStatus, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var devs []rawDevice
	if err := json.Unmarshal([]byte(raw), &devs); err != nil {
		return nil, fmt.Errorf("parse device json for node %q: %w", node, err)
	}
	out := make([]v1alpha1.DiskStatus, 0, len(devs))
	for _, d := range devs {
		path := "/dev/" + d.Name
		if loopDevices {
			// rook-discover reports loop-backed partitions as type "part"; a
			// bare loop device is rejected by the discover daemon itself.
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

// DeviceKey is the identity a Disk CR is named after. It prefers whatever the
// node reports that survives a reboot, because a Disk whose name changes is a
// Disk that drops out of every StoragePool listing it: the old name resolves to
// nothing and the device silently leaves the CephCluster.
//
// The prefixes keep the namespaces apart, so a serial can never collide with a
// WWN that happens to have the same text.
func DeviceKey(st v1alpha1.DiskStatus) string {
	switch {
	case st.StablePath != "":
		return st.StablePath
	case st.WWN != "":
		return "wwn:" + st.WWN
	case st.Serial != "":
		return "serial:" + st.Serial
	default:
		// Nothing stable was reported. Loop devices land here, and so do some
		// virtio disks; the name then moves if the kernel reorders them.
		return "path:" + st.Path
	}
}

// DiskName is a deterministic, DNS-1123-safe name for a (node, key) pair, where
// key comes from DeviceKey.
// The digest is a naming device, not a security control; sha256 is used anyway
// so the file needs no linter exemption for sha1.
func DiskName(node, key string) string {
	sum := sha256.Sum256([]byte(key))
	short := hex.EncodeToString(sum[:])[:10]
	base := strings.ToLower(node)
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, base)
	base = strings.Trim(base, "-")
	if base == "" {
		base = "node"
	}
	return fmt.Sprintf("%s-%s", base, short)
}
