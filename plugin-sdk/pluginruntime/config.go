package pluginruntime

import (
	"log/slog"
	"time"
)

// Config holds base configuration injected via environment variables by the plugin-worker.
type Config struct {
	ClusterID         string        `env:"FUNDAMENT_CLUSTER_ID,required,notEmpty"`
	InstallID         string        `env:"FUNDAMENT_INSTALL_ID,required,notEmpty"`
	OrganizationID    string        `env:"FUNDAMENT_ORGANIZATION_ID,required,notEmpty"`
	LogLevel          slog.Level    `env:"FUNP_LOG_LEVEL" envDefault:"info"`
	ReconcileInterval time.Duration `env:"FUNP_RECONCILE_INTERVAL" envDefault:"5m"`
}
