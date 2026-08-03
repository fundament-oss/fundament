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
	// AllowLoopDevices lets Ceph use loop-backed devices as OSDs and surfaces
	// them in the Disk inventory. For local/dev/CI clusters only (there are no
	// real disks); mirrors Rook's own allowLoopDevices/ROOK_ALLOW_LOOP_DEVICES.
	AllowLoopDevices bool `env:"ALLOW_LOOP_DEVICES" envDefault:"false"`
}

// LoadConfig parses the plugin configuration from the environment.
func LoadConfig() (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: "FUNP_"}); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
