package main

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config is the plugin's runtime configuration, injected by the plugin
// controller as FUNP_-prefixed environment variables.
type Config struct {
	RookChartVersion string `env:"ROOK_CHART_VERSION" envDefault:"v1.16.0"`
	RookNamespace    string `env:"ROOK_NAMESPACE" envDefault:"rook-ceph"`
	ClusterNamespace string `env:"CLUSTER_NAMESPACE" envDefault:"rook-ceph"`

	// Reef (v18.x) arm64 images segfault on startup; v19+ is required on Apple
	// Silicon.
	CephImage string `env:"CEPH_IMAGE" envDefault:"quay.io/ceph/ceph:v19.2.5"`

	// Quorum sizing. A single-node cluster needs count 1 and
	// AllowMultiplePerNode, or mons never reach quorum.
	MonCount             int64 `env:"MON_COUNT" envDefault:"3"`
	MgrCount             int64 `env:"MGR_COUNT" envDefault:"2"`
	AllowMultiplePerNode bool  `env:"ALLOW_MULTIPLE_PER_NODE" envDefault:"false"`

	// DevLoopDevices is an allowlist switch for local k3d development: only
	// /dev/loopNpN is discovered, and real disks are ignored entirely.
	DevLoopDevices bool `env:"DEV_LOOP_DEVICES" envDefault:"false"`
}

// LoadConfig parses the plugin configuration from the environment.
func LoadConfig() (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: "FUNP_"}); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
