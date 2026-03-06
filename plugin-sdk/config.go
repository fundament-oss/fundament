package pluginsdk

import (
	"log/slog"
	"time"
)

// Config holds base configuration injected via environment variables by the plugin-worker.
type Config struct {
	ClusterID         string        `env:"FUNDAMENT_CLUSTER_ID,required,notEmpty"`
	InstallID         string        `env:"FUNDAMENT_INSTALL_ID,required,notEmpty"`
	OrganizationID    string        `env:"FUNDAMENT_ORGANIZATION_ID,required,notEmpty"`
	JWTSecret         string        `env:"FUNDAMENT_JWT_SECRET"`
	APIURL            string        `env:"FUNDAMENT_API_URL"`
	LogLevel          slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	ReconcileInterval time.Duration `env:"RECONCILE_INTERVAL" envDefault:"5m"`
}
