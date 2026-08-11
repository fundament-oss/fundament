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
	ByID       string `json:"by-id"`
}

// loopPartition matches a partition on a loop device, e.g. /dev/loop0p1.
var loopPartition = regexp.MustCompile(`^/dev/loop\d+p\d+$`)

// ParseDiscoveredDevices converts a rook discover ConfigMap's device JSON into
// candidate Disk statuses.
//
// Normally only whole disks (type=="disk") are returned. In loop-device mode
// that is replaced -- not extended -- by loop-backed partitions. A k3d node
// exposes the host's real disks to every privileged container and they do reach
// the discover ConfigMap, so this filter is the only thing keeping them out of
// an OSD.
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
		path := d.ByID
		if path == "" {
			path = "/dev/" + d.Name
		}
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
		if d.Rotational {
			dt = v1alpha1.DiskTypeHDD
		}
		out = append(out, v1alpha1.DiskStatus{
			Node:       node,
			Path:       path,
			SizeBytes:  d.Size,
			Type:       dt,
			Rotational: d.Rotational,
			Model:      d.Model,
			Serial:     d.Serial,
			Available:  d.Empty && d.Filesystem == "",
		})
	}
	return out, nil
}

// DiskName is a deterministic, DNS-1123-safe name for a (node, path) pair.
// The digest is a naming device, not a security control; sha256 is used anyway
// so the file needs no linter exemption for sha1.
func DiskName(node, path string) string {
	sum := sha256.Sum256([]byte(path))
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
