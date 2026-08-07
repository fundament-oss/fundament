package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

type rawDevice struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Rotational int    `json:"rotational"`
	Type       string `json:"type"`
	Empty      bool   `json:"empty"`
	Filesystem string `json:"filesystem"`
	Vendor     string `json:"vendor"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	ByID       string `json:"by-id"`
}

// ParseDiscoveredDevices converts a rook discover ConfigMap's device JSON into
// candidate Disk statuses. Only whole disks (type=="disk") are returned; when
// allowLoopDevices is set, loop-backed devices (type=="loop") are also included
// for local/dev/CI clusters that have no real disks.
func ParseDiscoveredDevices(node string, raw string, allowLoopDevices bool) ([]v1alpha1.DiskStatus, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var devs []rawDevice
	if err := json.Unmarshal([]byte(raw), &devs); err != nil {
		return nil, fmt.Errorf("parse device json for node %q: %w", node, err)
	}
	out := make([]v1alpha1.DiskStatus, 0, len(devs))
	for _, d := range devs {
		if d.Type != "disk" && !(allowLoopDevices && d.Type == "loop") {
			continue
		}
		path := d.ByID
		if path == "" {
			path = "/dev/" + d.Name
		}
		dt := v1alpha1.DiskTypeSSD
		if d.Rotational == 1 {
			dt = v1alpha1.DiskTypeHDD
		}
		out = append(out, v1alpha1.DiskStatus{
			Node:       node,
			Path:       path,
			SizeBytes:  d.Size,
			Type:       dt,
			Rotational: d.Rotational == 1,
			Model:      d.Model,
			Serial:     d.Serial,
			Available:  d.Empty && d.Filesystem == "",
		})
	}
	return out, nil
}

// DiskName is a deterministic, DNS-1123-safe name for a (node, path) pair.
func DiskName(node, path string) string {
	sum := sha1.Sum([]byte(path))
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
